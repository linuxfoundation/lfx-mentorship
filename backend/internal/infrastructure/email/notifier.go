// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// pageSize is the largest page the member and task repositories serve.
const pageSize = 100

// mentorPageSize is the largest page the mentor management listing serves.
const mentorPageSize = 50

// dispatchTimeout bounds one notification's lookups and sends so a stuck dependency cannot hold shutdown.
const dispatchTimeout = 30 * time.Second

// sendConcurrency caps parallel sends per notification, so slow recipients cannot starve the rest of the deadline.
const sendConcurrency = 5

// maxConcurrentDispatches caps notifications resolving and sending at once across the notifier, so a
// bulk operation that notifies hundreds of mentees cannot exhaust the database pool or flood NATS.
const maxConcurrentDispatches = 10

var notifierTracer = otel.Tracer("email-notifier")

const (
	greetingFallback = "there"
	menteeFallback   = "A mentee"
	mentorFallback   = "A mentor"
	hrGreeting       = "HR team"
	reviewGreeting   = "LF Mentorship team"
)

const (
	// monthLayout renders a term as the legacy acceptance email did.
	monthLayout = "January 2006"
	// dateLayout renders a calendar date in task and graduation emails.
	dateLayout = "January 2, 2006"
)

// supportDeskURL is the LF support desk the legacy program rejection email pointed program admins at.
const supportDeskURL = "https://jira.linuxfoundation.org/servicedesk/customer/portal/4/group/31"

// selfServeMentorshipPath is the Self Serve lens where program admins, mentors, and mentees manage mentorships.
const selfServeMentorshipPath = "/mentorship"

// selfServeMentorInvitesPath is the Self Serve page where an invited mentor accepts or declines.
const selfServeMentorInvitesPath = selfServeMentorshipPath + "/mentor/invites"

// Sender delivers one rendered Message.
type Sender interface {
	Send(ctx context.Context, msg Message) (Receipt, error)
}

// NotifierConfig holds link targets and staff inboxes.
type NotifierConfig struct {
	PublicSiteURL string
	SelfServeURL  string
	HRInbox       string
	// ReviewInbox is the LF staff inbox that reviews submitted programs.
	ReviewInbox string
	// AllowedRecipients, when set, are the only addresses mail goes to; every other send is suppressed.
	AllowedRecipients []string
}

// Repositories are the lookups the Notifier needs to resolve recipients and template data.
type Repositories struct {
	Users        domain.UserRepository
	Profiles     domain.UserProfileRepository
	Programs     domain.ProgramRepository
	Terms        domain.ProgramTermRepository
	Members      domain.ProgramMemberRepository
	Applications domain.ApplicationRepository
	Tasks        domain.TaskRepository
}

// Notifier implements domain.Notifier by emailing through the email service.
// Sends run in the background so they never delay or fail the triggering request.
type Notifier struct {
	sender       Sender
	repos        Repositories
	publicURL    string
	selfServeURL string
	hrInbox      string
	reviewInbox  string
	// allowedRecipients holds lowercased addresses; nil sends to everyone.
	allowedRecipients map[string]bool
	logger            *slog.Logger
	wg                sync.WaitGroup
	// dispatches holds one slot per notification in progress, up to maxConcurrentDispatches.
	dispatches chan struct{}
}

var _ domain.Notifier = (*Notifier)(nil)

// NewNotifier returns a Notifier that sends through sender.
func NewNotifier(sender Sender, repos Repositories, cfg NotifierConfig, logger *slog.Logger) *Notifier {
	return &Notifier{
		sender:       sender,
		repos:        repos,
		publicURL:    strings.TrimRight(cfg.PublicSiteURL, "/"),
		selfServeURL: strings.TrimRight(cfg.SelfServeURL, "/"),
		hrInbox:      cfg.HRInbox,
		reviewInbox:  cfg.ReviewInbox,
		logger:       logger,
		dispatches:   make(chan struct{}, maxConcurrentDispatches),

		allowedRecipients: recipientSet(cfg.AllowedRecipients),
	}
}

// recipientSet lowercases addrs for case-insensitive matching; it is nil when addrs is empty.
func recipientSet(addrs []string) map[string]bool {
	if len(addrs) == 0 {
		return nil
	}
	set := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		set[strings.ToLower(a)] = true
	}
	return set
}

// recipientAllowed reports whether the lowercased addr may receive mail. A plus-addressed
// recipient (user+tag@domain) matches its listed base address, so test accounts need no listing.
func (n *Notifier) recipientAllowed(addr string) bool {
	if n.allowedRecipients == nil || n.allowedRecipients[addr] {
		return true
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	base, _, tagged := strings.Cut(addr[:at], "+")
	return tagged && n.allowedRecipients[base+addr[at:]]
}

// Wait blocks until in-flight notifications finish or ctx ends.
func (n *Notifier) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NotifyMentorInvited emails the invited mentor a link to accept or decline.
func (n *Notifier) NotifyMentorInvited(ctx context.Context, programID, userID, token string) {
	n.dispatch(ctx, templateMentorInvited, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		user, to, err := n.recipient(ctx, userID)
		if err != nil {
			return nil, err
		}
		msg, err := render(templateMentorInvited, to, "You've been invited to mentor for "+program.Name, templateData{
			RecipientName: firstName(user),
			ProgramName:   program.Name,
			ProgramURL:    n.programURL(program),
			InviteURL:     n.selfServeURL + selfServeMentorInvitesPath + "?token=" + url.QueryEscape(token),
			LFID:          deref(user.LFID),
		})
		return []Message{msg}, err
	})
}

// NotifyMentorDeclined tells the mentor a program admin declined their request.
func (n *Notifier) NotifyMentorDeclined(ctx context.Context, programID, userID string) {
	n.dispatch(ctx, templateMentorDeclined, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		user, to, err := n.recipient(ctx, userID)
		if err != nil {
			return nil, err
		}
		msg, err := render(templateMentorDeclined, to, "Your mentor request for "+program.Name+" was declined", templateData{
			RecipientName: firstName(user),
			ProgramName:   program.Name,
			ProgramURL:    n.programURL(program),
		})
		return []Message{msg}, err
	})
}

// NotifyAdminMentorAccepted tells every active program admin that an invited mentor accepted.
func (n *Notifier) NotifyAdminMentorAccepted(ctx context.Context, programID, userID string) {
	n.notifyAdminsOfMentor(ctx, templateAdminMentorAccepted, programID, userID, func(mentor, program string) string {
		return mentor + " accepted your mentor invitation for " + program
	})
}

// NotifyAdminMentorDeclined tells every active program admin that an invited mentor declined.
func (n *Notifier) NotifyAdminMentorDeclined(ctx context.Context, programID, userID string) {
	n.notifyAdminsOfMentor(ctx, templateAdminMentorDeclined, programID, userID, func(mentor, program string) string {
		return mentor + " declined your mentor invitation for " + program
	})
}

// NotifyAdminMentorRequested tells every active program admin that a mentor asked to join the program.
func (n *Notifier) NotifyAdminMentorRequested(ctx context.Context, programID, userID string) {
	n.notifyAdminsOfMentor(ctx, templateAdminMentorRequested, programID, userID, func(mentor, program string) string {
		return mentor + " requested to mentor for " + program
	})
}

// NotifyAdminMentorWithdrew tells every active program admin that a mentor withdrew their request to join.
func (n *Notifier) NotifyAdminMentorWithdrew(ctx context.Context, programID, userID string) {
	n.notifyAdminsOfMentor(ctx, templateAdminMentorWithdrew, programID, userID, func(mentor, program string) string {
		return mentor + " withdrew their mentor request for " + program
	})
}

// notifyAdminsOfMentor tells every active program admin what a mentor did; subject builds the
// subject line from the mentor's and the program's names.
func (n *Notifier) notifyAdminsOfMentor(ctx context.Context, name, programID, userID string, subject func(mentor, program string) string) {
	n.dispatch(ctx, name, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		mentor, err := n.repos.Users.GetByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get mentor: %w", err)
		}
		admins, err := n.activeAdmins(ctx, program.ID, name)
		if err != nil {
			return nil, err
		}
		mentorName := fullName(mentor, mentorFallback)
		return adminMessages(admins, name, subject(mentorName, program.Name), templateData{
			ProgramName: program.Name,
			ManageURL:   n.manageURL(),
			MentorName:  mentorName,
			MentorEmail: n.mentorEmail(ctx, programID, mentor),
		})
	})
}

// mentorEmail is the address on the mentor's program_members row, which records the email the
// mentor asked or was invited with, falling back to their account email. A mentor-role
// application has no member row, and a failed lookup must not hold back the notice.
func (n *Notifier) mentorEmail(ctx context.Context, programID string, mentor *models.User) string {
	member, err := n.repos.Members.FindByProgramUserAndType(ctx, programID, mentor.ID, models.MemberTypeMentor)
	switch {
	case err == nil:
		if email := strings.TrimSpace(deref(member.Email)); email != "" {
			return email
		}
	case !errors.Is(err, domain.ErrProgramMemberNotFound):
		n.logger.WarnContext(ctx, "mentor member email unavailable", "user_id", mentor.ID, "error", err)
	}
	return strings.TrimSpace(deref(mentor.Email))
}

// NotifyProgramSubmitted tells every active program admin their program is in review, and the
// LF staff review inbox that it awaits a decision.
func (n *Notifier) NotifyProgramSubmitted(ctx context.Context, programID, submitterID string) {
	n.dispatch(ctx, templateProgramSubmitted, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		admins, err := n.activeAdmins(ctx, program.ID, templateProgramSubmitted)
		if err != nil {
			return nil, err
		}
		return adminMessages(admins, templateProgramSubmitted, "We received your submission of "+program.Name, templateData{
			ProgramName: program.Name,
			ManageURL:   n.manageURL(),
		})
	})
	n.dispatch(ctx, templateProgramReview, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		data := templateData{
			RecipientName: reviewGreeting,
			ProgramName:   program.Name,
			ManageURL:     n.manageURL(),
		}
		// The submitter is context for the reviewer; a failed lookup must not hold back the notice.
		if submitterID != "" {
			submitter, err := n.repos.Users.GetByID(ctx, submitterID)
			if err != nil {
				n.logger.WarnContext(ctx, "program submitter unavailable", "user_id", submitterID, "error", err)
			} else {
				data.SubmitterName = fullName(submitter, deref(submitter.LFID))
				data.SubmitterEmail = strings.TrimSpace(deref(submitter.Email))
			}
		}
		msg, err := render(templateProgramReview, n.reviewInbox, "Mentorship program submitted for review: "+program.Name, data)
		return []Message{msg}, err
	})
}

// NotifyProgramDecided tells every active program admin their program was published or rejected.
func (n *Notifier) NotifyProgramDecided(ctx context.Context, programID string, status models.ProgramStatus) {
	var name, verdict string
	switch status {
	case models.ProgramStatusPublished:
		name, verdict = templateProgramApproved, " has been approved"
	case models.ProgramStatusRejected:
		name, verdict = templateProgramRejected, " was not approved"
	default:
		n.logger.WarnContext(ctx, "program decision has no email", "program_id", programID, "status", status)
		return
	}
	n.dispatch(ctx, name, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		admins, err := n.activeAdmins(ctx, program.ID, name)
		if err != nil {
			return nil, err
		}
		return adminMessages(admins, name, program.Name+verdict, templateData{
			ProgramName: program.Name,
			ProgramURL:  n.programURL(program),
			ManageURL:   n.manageURL(),
			SupportURL:  supportDeskURL,
		})
	})
}

// NotifyAdminTasksSubmitted tells every active program admin that a mentee's application is ready for review.
func (n *Notifier) NotifyAdminTasksSubmitted(ctx context.Context, applicationID string) {
	n.dispatch(ctx, templateAdminTasksSubmitted, func(ctx context.Context) ([]Message, error) {
		app, term, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		mentee, err := n.repos.Users.GetByID(ctx, app.UserID)
		if err != nil {
			return nil, fmt.Errorf("get mentee: %w", err)
		}
		admins, err := n.activeAdmins(ctx, program.ID, templateAdminTasksSubmitted)
		if err != nil {
			return nil, err
		}
		return adminMessages(admins, templateAdminTasksSubmitted, "New mentee application to review for "+program.Name, templateData{
			ProgramName: program.Name,
			ManageURL:   n.manageURL(),
			TermName:    term.Name,
			MenteeName:  fullName(mentee, menteeFallback),
		})
	})
}

// NotifyMenteeAccepted congratulates the mentee and tells every active program admin.
func (n *Notifier) NotifyMenteeAccepted(ctx context.Context, applicationID, attendanceType string) {
	attendance := attendanceLabel(models.AttendanceType(attendanceType))
	n.dispatch(ctx, templateMenteeAccepted, func(ctx context.Context) ([]Message, error) {
		app, term, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		user, to, err := n.recipient(ctx, app.UserID)
		if err != nil {
			return nil, err
		}
		msg, err := render(templateMenteeAccepted, to, "Congratulations! You've been accepted to "+program.Name, templateData{
			RecipientName:  firstName(user),
			ProgramName:    program.Name,
			ManageURL:      n.manageURL(),
			TermName:       term.Name,
			AttendanceType: attendance,
		})
		return []Message{msg}, err
	})
	n.dispatch(ctx, templateAdminMenteeAccepted, func(ctx context.Context) ([]Message, error) {
		app, term, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		mentee, err := n.repos.Users.GetByID(ctx, app.UserID)
		if err != nil {
			return nil, fmt.Errorf("get mentee: %w", err)
		}
		menteeName := fullName(mentee, menteeFallback)
		subject := menteeName + " has been accepted to " + program.Name
		admins, err := n.activeAdmins(ctx, program.ID, templateAdminMenteeAccepted)
		if err != nil {
			return nil, err
		}
		data := templateData{
			ProgramName:    program.Name,
			ManageURL:      n.manageURL(),
			TermName:       term.Name,
			TermPeriod:     termPeriod(term, monthLayout),
			MenteeName:     menteeName,
			MenteeEmail:    strings.TrimSpace(deref(mentee.Email)),
			AttendanceType: attendance,
			ProgramAdmins:  adminContacts(admins),
		}
		msgs, err := adminMessages(admins, templateAdminMenteeAccepted, subject, data)
		if err != nil {
			return nil, err
		}
		data.RecipientName = hrGreeting
		data.HRNotice = true
		data.MenteeCountry = n.menteeCountry(ctx, app.UserID)
		hr, err := render(templateAdminMenteeAccepted, n.hrInbox, subject, data)
		if err != nil {
			return nil, err
		}
		// HR goes first: dispatch keeps the first message per address, and an HR inbox that is
		// also an admin must still get the HR notice with the country of residence.
		return append([]Message{hr}, msgs...), nil
	})
}

// NotifyMenteeApplicationReceived confirms the application to the mentee and lists the prerequisite tasks to complete.
func (n *Notifier) NotifyMenteeApplicationReceived(ctx context.Context, applicationID string) {
	n.dispatch(ctx, templateMenteeApplicationReceived, func(ctx context.Context) ([]Message, error) {
		app, _, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		user, to, err := n.recipient(ctx, app.UserID)
		if err != nil {
			return nil, err
		}
		tasks, err := n.applicationTasks(ctx, app.ID)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, task := range tasks {
			if task.Category == nil || *task.Category != models.TaskCategoryPrerequisite {
				continue
			}
			if name := strings.TrimSpace(deref(task.Name)); name != "" {
				names = append(names, name)
			}
		}
		msg, err := render(templateMenteeApplicationReceived, to, "We received your application to "+program.Name, templateData{
			RecipientName: firstName(user),
			ProgramName:   program.Name,
			ManageURL:     n.manageURL(),
			Tasks:         names,
		})
		return []Message{msg}, err
	})
}

// NotifyMenteeDeclined tells the mentee their application was declined.
func (n *Notifier) NotifyMenteeDeclined(ctx context.Context, applicationID string) {
	n.dispatch(ctx, templateMenteeDeclined, func(ctx context.Context) ([]Message, error) {
		app, _, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		user, to, err := n.recipient(ctx, app.UserID)
		if err != nil {
			return nil, err
		}
		msg, err := render(templateMenteeDeclined, to, "An update on your application to "+program.Name, templateData{
			RecipientName: firstName(user),
			ProgramName:   program.Name,
			ManageURL:     n.manageURL(),
		})
		return []Message{msg}, err
	})
}

// NotifyMenteeWithdrawn confirms the withdrawal to the mentee and tells every active program admin.
func (n *Notifier) NotifyMenteeWithdrawn(ctx context.Context, applicationID string) {
	n.dispatch(ctx, templateMenteeWithdrawn, func(ctx context.Context) ([]Message, error) {
		app, _, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		user, to, err := n.recipient(ctx, app.UserID)
		if err != nil {
			return nil, err
		}
		msg, err := render(templateMenteeWithdrawn, to, "Your application to "+program.Name+" has been withdrawn", templateData{
			RecipientName: firstName(user),
			ProgramName:   program.Name,
			ProgramURL:    n.programURL(program),
		})
		return []Message{msg}, err
	})
	n.dispatch(ctx, templateAdminMenteeWithdrawn, func(ctx context.Context) ([]Message, error) {
		app, term, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		mentee, err := n.repos.Users.GetByID(ctx, app.UserID)
		if err != nil {
			return nil, fmt.Errorf("get mentee: %w", err)
		}
		admins, err := n.activeAdmins(ctx, program.ID, templateAdminMenteeWithdrawn)
		if err != nil {
			return nil, err
		}
		menteeName := fullName(mentee, menteeFallback)
		return adminMessages(admins, templateAdminMenteeWithdrawn, menteeName+"'s application to "+program.Name+" was withdrawn", templateData{
			ProgramName: program.Name,
			ManageURL:   n.manageURL(),
			TermName:    term.Name,
			MenteeName:  menteeName,
		})
	})
}

// NotifyMenteeGraduated sends the mentee a letter certifying they completed the program.
func (n *Notifier) NotifyMenteeGraduated(ctx context.Context, applicationID string) {
	n.dispatch(ctx, templateMenteeGraduated, func(ctx context.Context) ([]Message, error) {
		app, term, program, err := n.applicationContext(ctx, applicationID)
		if err != nil {
			return nil, err
		}
		user, to, err := n.recipient(ctx, app.UserID)
		if err != nil {
			return nil, err
		}
		mentors, err := n.mentorNames(ctx, program.ID)
		if err != nil {
			return nil, err
		}
		msg, err := render(templateMenteeGraduated, to, "Your LFX Mentorship certificate of completion for "+program.Name, templateData{
			Letter:             true,
			IssuedDate:         time.Now().UTC().Format(dateLayout),
			MenteeName:         fullName(user, deref(user.LFID)),
			ProgramName:        program.Name,
			ProgramDescription: strings.TrimSpace(deref(program.Description)),
			TermPeriod:         termPeriod(term, dateLayout),
			MentorNames:        mentors,
		})
		return []Message{msg}, err
	})
}

// NotifyMenteeTaskAssigned tells the task's assignee a reviewer assigned them a task.
func (n *Notifier) NotifyMenteeTaskAssigned(ctx context.Context, taskID string) {
	n.dispatch(ctx, templateMenteeTaskAssigned, func(ctx context.Context) ([]Message, error) {
		task, err := n.repos.Tasks.GetByID(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("get task: %w", err)
		}
		if task.ApplicationID == nil {
			return nil, fmt.Errorf("task %s has no application", taskID)
		}
		_, _, program, err := n.applicationContext(ctx, *task.ApplicationID)
		if err != nil {
			return nil, err
		}
		user, to, err := n.recipient(ctx, task.AssigneeID)
		if err != nil {
			return nil, err
		}
		msg, err := render(templateMenteeTaskAssigned, to, "You have a new task for "+program.Name, templateData{
			RecipientName: firstName(user),
			ProgramName:   program.Name,
			ManageURL:     n.manageURL(),
			TaskName:      deref(task.Name),
			TaskCreatedOn: task.CreatedOn.UTC().Format(dateLayout),
			TaskDueDate:   dueDate(task.DueDate),
		})
		return []Message{msg}, err
	})
}

// menteeCountry returns the country of residence on the user's mentee profile, or "" when unknown.
// HR uses it for stipend verification; a failed lookup must not hold back the notice.
func (n *Notifier) menteeCountry(ctx context.Context, userID string) string {
	profiles, _, err := n.repos.Profiles.List(ctx, models.UserProfileFilter{
		UserID:      userID,
		ProfileType: string(models.UserProfileTypeMentee),
		Limit:       1,
	})
	if err != nil {
		n.logger.WarnContext(ctx, "mentee country unavailable", "user_id", userID, "error", err)
		return ""
	}
	if len(profiles) == 0 || len(profiles[0].Address) == 0 {
		return ""
	}
	var address struct {
		Country string `json:"country"`
	}
	if err := json.Unmarshal(profiles[0].Address, &address); err != nil {
		n.logger.WarnContext(ctx, "mentee profile address is not an object", "user_id", userID, "error", err)
		return ""
	}
	return countryName(address.Country)
}

// countryName expands an ISO 3166-1 alpha-2 code to "English name (CODE)"; the code keeps it
// unambiguous where CLDR names differ from the legacy table. Any other value is returned as stored.
func countryName(country string) string {
	country = strings.TrimSpace(country)
	if len(country) != 2 {
		return country
	}
	region, err := language.ParseRegion(country)
	if err != nil || !region.IsCountry() {
		return country
	}
	return display.English.Regions().Name(region) + " (" + region.String() + ")"
}

// dispatch resolves and sends a notification in the background, logging any failure.
func (n *Notifier) dispatch(ctx context.Context, notification string, build func(context.Context) ([]Message, error)) {
	// The triggering request's context is cancelled when it returns; keep its values (trace) only.
	ctx = context.WithoutCancel(ctx)
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		// The deadline starts once a slot is free, so a queued notification is not timed out for waiting.
		n.dispatches <- struct{}{}
		defer func() { <-n.dispatches }()
		ctx, cancel := context.WithTimeout(ctx, dispatchTimeout)
		defer cancel()
		// Chi's recoverer does not cover this goroutine; a panic here would take down the API.
		defer func() {
			if r := recover(); r != nil {
				n.logger.ErrorContext(ctx, "email notification panicked", "notification", notification, "panic", r)
			}
		}()
		ctx, span := notifierTracer.Start(ctx, "email.notify "+notification)
		defer span.End()

		msgs, err := build(ctx)
		if err != nil {
			span.RecordError(err)
			n.logger.ErrorContext(ctx, "email notification not sent", "notification", notification, "error", err)
			return
		}
		if len(msgs) == 0 {
			n.logger.WarnContext(ctx, "email notification has no recipients", "notification", notification)
			return
		}
		seen := make(map[string]bool, len(msgs))
		slots := make(chan struct{}, sendConcurrency)
		var sends sync.WaitGroup
		for _, msg := range msgs {
			addr := strings.ToLower(msg.To)
			if seen[addr] {
				continue
			}
			seen[addr] = true
			if !n.recipientAllowed(addr) {
				n.logger.InfoContext(ctx, "email notification suppressed", "notification", notification)
				continue
			}
			slots <- struct{}{}
			sends.Add(1)
			go func() {
				defer sends.Done()
				defer func() { <-slots }()
				defer func() {
					if r := recover(); r != nil {
						n.logger.ErrorContext(ctx, "email notification panicked", "notification", notification, "panic", r)
					}
				}()
				receipt, err := n.sender.Send(ctx, msg)
				if err != nil {
					span.RecordError(err)
					n.logger.ErrorContext(ctx, "email notification not sent", "notification", notification, "error", err)
					return
				}
				n.logger.InfoContext(ctx, "email notification sent", "notification", notification, "email_id", receipt.EmailID)
			}()
		}
		sends.Wait()
	}()
}

// recipientUser is a resolved user with a usable email address.
type recipientUser struct {
	user  *models.User
	email string
}

// activeMembers lists every active member of the program with the given type.
func (n *Notifier) activeMembers(ctx context.Context, programID string, memberType models.MemberType) ([]*models.ProgramMember, error) {
	var members []*models.ProgramMember
	for {
		page, meta, err := n.repos.Members.ListByProgram(ctx, programID, models.ProgramMemberFilter{
			Limit:      pageSize,
			Offset:     len(members),
			MemberType: string(memberType),
			Status:     string(models.ProgramMemberStatusActive),
		})
		if err != nil {
			return nil, err
		}
		members = append(members, page...)
		if len(page) == 0 || meta == nil || len(members) >= meta.Total {
			return members, nil
		}
	}
}

// applicationTasks lists every task on the application, a page at a time.
func (n *Notifier) applicationTasks(ctx context.Context, applicationID string) ([]*models.Task, error) {
	var tasks []*models.Task
	for {
		page, meta, err := n.repos.Tasks.ListByApplication(ctx, applicationID, models.TaskFilter{Limit: pageSize, Offset: len(tasks)})
		if err != nil {
			return nil, fmt.Errorf("list application tasks: %w", err)
		}
		tasks = append(tasks, page...)
		if len(page) == 0 || meta == nil || len(tasks) >= meta.Total {
			return tasks, nil
		}
	}
}

// mentorNames lists the program's active mentors by name, or LFID when unnamed. The mentor
// management listing joins each mentor's user, so this costs one query per page, not per mentor.
func (n *Notifier) mentorNames(ctx context.Context, programID string) ([]string, error) {
	var rows []*models.ProgramMentorManagementRow
	for {
		page, meta, err := n.repos.Members.ListMentorManagement(ctx, programID, models.ProgramMemberFilter{
			Limit:  mentorPageSize,
			Offset: len(rows),
			Status: string(models.ProgramMemberStatusActive),
		})
		if err != nil {
			return nil, fmt.Errorf("list program mentors: %w", err)
		}
		rows = append(rows, page...)
		if len(page) == 0 || meta == nil || len(rows) >= meta.Total {
			break
		}
	}
	var names []string
	for _, row := range rows {
		name := strings.TrimSpace(deref(row.Name))
		if name == "" {
			name = strings.TrimSpace(deref(row.Username))
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// activeAdmins resolves every active admin of the program, skipping any without an email address.
func (n *Notifier) activeAdmins(ctx context.Context, programID, notification string) ([]recipientUser, error) {
	members, err := n.activeMembers(ctx, programID, models.MemberTypeProgramAdmin)
	if err != nil {
		return nil, fmt.Errorf("list program admins: %w", err)
	}
	var admins []recipientUser
	for _, m := range members {
		user, to, err := n.recipient(ctx, m.UserID)
		if err != nil {
			n.logger.WarnContext(ctx, "skipping program admin email", "notification", notification, "user_id", m.UserID, "error", err)
			continue
		}
		admins = append(admins, recipientUser{user: user, email: to})
	}
	return admins, nil
}

// adminMessages renders one message per admin.
func adminMessages(admins []recipientUser, name, subject string, data templateData) ([]Message, error) {
	msgs := make([]Message, 0, len(admins))
	for _, admin := range admins {
		data.RecipientName = firstName(admin.user)
		msg, err := render(name, admin.email, subject, data)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// adminContacts lists admins as "Name (email)", as the legacy acceptance notice did for HR.
func adminContacts(admins []recipientUser) []string {
	contacts := make([]string, 0, len(admins))
	for _, admin := range admins {
		contact := admin.email
		if name := fullName(admin.user, ""); name != "" {
			contact = name + " (" + admin.email + ")"
		}
		contacts = append(contacts, contact)
	}
	return contacts
}

func (n *Notifier) recipient(ctx context.Context, userID string) (*models.User, string, error) {
	user, err := n.repos.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("get user %s: %w", userID, err)
	}
	to := strings.TrimSpace(deref(user.Email))
	if to == "" {
		return nil, "", fmt.Errorf("user %s has no email address", userID)
	}
	return user, to, nil
}

func (n *Notifier) applicationContext(ctx context.Context, applicationID string) (*models.Application, *models.ProgramTerm, *models.Program, error) {
	app, err := n.repos.Applications.GetByID(ctx, applicationID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get application: %w", err)
	}
	term, err := n.repos.Terms.GetByID(ctx, app.ProgramTermID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get program term: %w", err)
	}
	program, err := n.repos.Programs.GetByID(ctx, term.ProgramID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get program: %w", err)
	}
	return app, term, program, nil
}

func (n *Notifier) programURL(program *models.Program) string {
	ref := program.Slug
	if ref == "" {
		ref = program.ID
	}
	return n.publicURL + "/programs/" + url.PathEscape(ref)
}

func (n *Notifier) manageURL() string {
	return n.selfServeURL + selfServeMentorshipPath
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// firstName is the greeting name, matching the legacy templates' FNAME.
func firstName(user *models.User) string {
	for _, s := range []*string{user.GivenName, user.Name} {
		if v := strings.TrimSpace(deref(s)); v != "" {
			return v
		}
	}
	return greetingFallback
}

func fullName(user *models.User, fallback string) string {
	if v := strings.TrimSpace(deref(user.Name)); v != "" {
		return v
	}
	var parts []string
	for _, s := range []*string{user.GivenName, user.FamilyName} {
		if v := strings.TrimSpace(deref(s)); v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return fallback
}

// termPeriod renders the term's start and end in layout as "start – end", or "" when either is unset.
func termPeriod(term *models.ProgramTerm, layout string) string {
	if term.StartDateTime == nil || term.EndDateTime == nil {
		return ""
	}
	return term.StartDateTime.UTC().Format(layout) + " – " + term.EndDateTime.UTC().Format(layout)
}

// dueDate renders a task's YYYY-MM-DD due date in dateLayout; any other value is returned as stored.
func dueDate(value *string) string {
	d, err := time.Parse(time.DateOnly, deref(value))
	if err != nil {
		return deref(value)
	}
	return d.Format(dateLayout)
}

func attendanceLabel(t models.AttendanceType) string {
	switch t {
	case models.AttendanceTypeFullTime:
		return "full-time"
	case models.AttendanceTypePartTime:
		return "part-time"
	default:
		return ""
	}
}
