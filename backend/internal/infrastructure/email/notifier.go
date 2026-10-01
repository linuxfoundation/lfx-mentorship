// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"context"
	"encoding/json"
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

// maxProgramAdmins caps admin recipients per notification; the repository pages at 100.
const maxProgramAdmins = 100

// dispatchTimeout bounds one notification's lookups and sends so a stuck dependency cannot hold shutdown.
const dispatchTimeout = 30 * time.Second

var notifierTracer = otel.Tracer("email-notifier")

const (
	greetingFallback = "there"
	menteeFallback   = "A mentee"
	mentorFallback   = "A mentor"
	hrGreeting       = "HR team"
)

// selfServeMentorshipPath is the Self Serve lens where program admins, mentors, and mentees manage mentorships.
const selfServeMentorshipPath = "/mentorship"

// Sender delivers one rendered Message.
type Sender interface {
	Send(ctx context.Context, msg Message) (Receipt, error)
}

// NotifierConfig holds link targets and staff inboxes.
type NotifierConfig struct {
	PublicSiteURL string
	SelfServeURL  string
	HRInbox       string
}

// Repositories are the lookups the Notifier needs to resolve recipients and template data.
type Repositories struct {
	Users        domain.UserRepository
	Profiles     domain.UserProfileRepository
	Programs     domain.ProgramRepository
	Terms        domain.ProgramTermRepository
	Members      domain.ProgramMemberRepository
	Applications domain.ApplicationRepository
}

// Notifier implements domain.Notifier by emailing through the email service.
// Sends run in the background so they never delay or fail the triggering request.
type Notifier struct {
	sender       Sender
	repos        Repositories
	publicURL    string
	selfServeURL string
	hrInbox      string
	logger       *slog.Logger
	wg           sync.WaitGroup
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
		logger:       logger,
	}
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
			InviteURL:     n.publicURL + "/mentor-invite?token=" + url.QueryEscape(token),
			LFID:          deref(user.LFID),
		})
		return []Message{msg}, err
	})
}

// NotifyMentorDeclined tells the mentor a program admin declined them.
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
	n.dispatch(ctx, templateAdminMentorAccepted, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		mentor, err := n.repos.Users.GetByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get mentor: %w", err)
		}
		admins, err := n.activeAdmins(ctx, program.ID, templateAdminMentorAccepted)
		if err != nil {
			return nil, err
		}
		mentorName := fullName(mentor, mentorFallback)
		return adminMessages(admins, templateAdminMentorAccepted, mentorName+" accepted your mentor invitation for "+program.Name, templateData{
			ProgramName: program.Name,
			ManageURL:   n.manageURL(),
			MentorName:  mentorName,
		})
	})
}

// NotifyAdminMentorDeclined tells every active program admin that an invited mentor declined.
func (n *Notifier) NotifyAdminMentorDeclined(ctx context.Context, programID, userID string) {
	n.dispatch(ctx, templateAdminMentorDeclined, func(ctx context.Context) ([]Message, error) {
		program, err := n.repos.Programs.GetByID(ctx, programID)
		if err != nil {
			return nil, fmt.Errorf("get program: %w", err)
		}
		mentor, err := n.repos.Users.GetByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get mentor: %w", err)
		}
		mentorName := fullName(mentor, mentorFallback)
		admins, err := n.activeAdmins(ctx, program.ID, templateAdminMentorDeclined)
		if err != nil {
			return nil, err
		}
		return adminMessages(admins, templateAdminMentorDeclined, mentorName+" declined your mentor invitation for "+program.Name, templateData{
			ProgramName: program.Name,
			ManageURL:   n.manageURL(),
			MentorName:  mentorName,
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
			TermPeriod:     termPeriod(term),
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
		return append(msgs, hr), nil
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
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dispatchTimeout)
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
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
		for _, msg := range msgs {
			addr := strings.ToLower(msg.To)
			if seen[addr] {
				continue
			}
			seen[addr] = true
			receipt, err := n.sender.Send(ctx, msg)
			if err != nil {
				span.RecordError(err)
				n.logger.ErrorContext(ctx, "email notification not sent", "notification", notification, "error", err)
				continue
			}
			n.logger.InfoContext(ctx, "email notification sent", "notification", notification, "email_id", receipt.EmailID)
		}
	}()
}

// recipientUser is a resolved user with a usable email address.
type recipientUser struct {
	user  *models.User
	email string
}

// activeAdmins resolves the program's active admins, skipping any without an email address.
func (n *Notifier) activeAdmins(ctx context.Context, programID, notification string) ([]recipientUser, error) {
	members, _, err := n.repos.Members.ListByProgram(ctx, programID, models.ProgramMemberFilter{
		Limit:      maxProgramAdmins,
		MemberType: string(models.MemberTypeProgramAdmin),
		Status:     string(models.ProgramMemberStatusActive),
	})
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

// termPeriod renders the term as "Month Year – Month Year", as the legacy acceptance email did.
func termPeriod(term *models.ProgramTerm) string {
	if term.StartDateTime == nil || term.EndDateTime == nil {
		return ""
	}
	return term.StartDateTime.UTC().Format("January 2006") + " – " + term.EndDateTime.UTC().Format("January 2006")
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
