// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

const (
	testPublicSiteURL = "https://mentorship.example.org"
	testSelfServeURL  = "https://app.example.org"
	testHRInbox       = "hr@linuxfoundation.org"
	testReviewInbox   = "review@linuxfoundation.org"
	// testAdminProgramURL is the fixture program's page for its program admins.
	testAdminProgramURL = testSelfServeURL + "/mentorship/admin/p1"
)

type senderStub struct {
	mu          sync.Mutex
	sent        []Message
	failFor     map[string]bool
	release     chan struct{}
	delay       time.Duration
	inFlight    int
	maxInFlight int
}

func (s *senderStub) Send(_ context.Context, msg Message) (Receipt, error) {
	s.mu.Lock()
	s.inFlight++
	s.maxInFlight = max(s.maxInFlight, s.inFlight)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}()
	if s.release != nil {
		<-s.release
	}
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failFor[msg.To] {
		return Receipt{}, ErrRejected
	}
	s.sent = append(s.sent, msg)
	return Receipt{EmailID: "e-" + msg.To}, nil
}

type userRepoStub struct {
	domain.UserRepository
	users map[string]*models.User
}

func (r *userRepoStub) GetByID(_ context.Context, id string) (*models.User, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}

type programRepoStub struct {
	domain.ProgramRepository
	program *models.Program
}

func (r *programRepoStub) GetByID(_ context.Context, id string) (*models.Program, error) {
	if r.program != nil && r.program.ID == id {
		return r.program, nil
	}
	return nil, domain.ErrProgramNotFound
}

type termRepoStub struct {
	domain.ProgramTermRepository
	term *models.ProgramTerm
}

func (r *termRepoStub) GetByID(context.Context, string) (*models.ProgramTerm, error) {
	return r.term, nil
}

type memberRepoStub struct {
	domain.ProgramMemberRepository
	members       []*models.ProgramMember
	filters       []models.ProgramMemberFilter
	mentors       []*models.ProgramMentorManagementRow
	mentorFilters []models.ProgramMemberFilter
	// mentorRow is the mentor's own program_members row; nil reports it not found unless mentorErr is set.
	mentorRow *models.ProgramMember
	mentorErr error
}

func (r *memberRepoStub) ListByProgram(_ context.Context, _ string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	r.filters = append(r.filters, filter)
	start := min(filter.Offset, len(r.members))
	end := min(start+filter.Limit, len(r.members))
	return r.members[start:end], &models.PaginationMeta{Total: len(r.members), Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (r *memberRepoStub) ListMentorManagement(_ context.Context, _ string, filter models.ProgramMemberFilter) ([]*models.ProgramMentorManagementRow, *models.PaginationMeta, error) {
	r.mentorFilters = append(r.mentorFilters, filter)
	start := min(filter.Offset, len(r.mentors))
	end := min(start+filter.Limit, len(r.mentors))
	return r.mentors[start:end], &models.PaginationMeta{Total: len(r.mentors), Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (r *memberRepoStub) FindByProgramUserAndType(context.Context, string, string, models.MemberType) (*models.ProgramMember, error) {
	if r.mentorErr != nil {
		return nil, r.mentorErr
	}
	if r.mentorRow == nil {
		return nil, domain.ErrProgramMemberNotFound
	}
	return r.mentorRow, nil
}

type applicationRepoStub struct {
	domain.ApplicationRepository
	app *models.Application
}

func (r *applicationRepoStub) GetByID(context.Context, string) (*models.Application, error) {
	return r.app, nil
}

type taskRepoStub struct {
	domain.TaskRepository
	tasks []*models.Task
	err   error
}

func (r *taskRepoStub) GetByID(_ context.Context, id string) (*models.Task, error) {
	for _, t := range r.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, domain.ErrTaskNotFound
}

func (r *taskRepoStub) ListByApplication(_ context.Context, _ string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	if r.err != nil {
		return nil, nil, r.err
	}
	start := min(filter.Offset, len(r.tasks))
	end := min(start+filter.Limit, len(r.tasks))
	return r.tasks[start:end], &models.PaginationMeta{Total: len(r.tasks), Limit: filter.Limit, Offset: filter.Offset}, nil
}

type profileRepoStub struct {
	domain.UserProfileRepository
	address json.RawMessage
	err     error
}

func (r *profileRepoStub) List(_ context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
	if r.err != nil {
		return nil, nil, r.err
	}
	if filter.ProfileType != string(models.UserProfileTypeMentee) || r.address == nil {
		return nil, &models.PaginationMeta{}, nil
	}
	return []*models.UserProfile{{UserID: filter.UserID, ProfileType: filter.ProfileType, Address: r.address}}, &models.PaginationMeta{}, nil
}

func strPtr(s string) *string { return &s }

func timePtr(t time.Time) *time.Time { return &t }

func byRecipient(msgs []Message) map[string]Message {
	out := make(map[string]Message, len(msgs))
	for _, m := range msgs {
		out[m.To] = m
	}
	return out
}

type fixture struct {
	sender   *senderStub
	members  *memberRepoStub
	users    *userRepoStub
	profiles *profileRepoStub
	tasks    *taskRepoStub
	program  *models.Program
	n        *Notifier
}

func newFixture() *fixture {
	f := &fixture{
		sender: &senderStub{},
		users: &userRepoStub{users: map[string]*models.User{
			"mentor":  {ID: "mentor", Email: strPtr("mentor@linuxfoundation.org"), Name: strPtr("Mo Mentor"), LFID: strPtr("momentor")},
			"inviter": {ID: "inviter", Name: strPtr("Ada Admin")},
			"mentee":  {ID: "mentee", Email: strPtr("mentee@linuxfoundation.org"), GivenName: strPtr("Mia"), FamilyName: strPtr("Mentee")},
			"admin1":  {ID: "admin1", Email: strPtr("admin1@linuxfoundation.org")},
			"admin2":  {ID: "admin2", Email: strPtr("admin2@linuxfoundation.org")},
			"noaddr":  {ID: "noaddr"},
		}},
		members:  &memberRepoStub{},
		profiles: &profileRepoStub{address: json.RawMessage(`{"country":"IN","city":"Pune"}`)},
		tasks:    &taskRepoStub{},
		program:  &models.Program{ID: "p1", Name: "Kernel <Dev>", Slug: "kernel-dev", Description: strPtr("Work on <drivers>.")},
	}
	f.n = NewNotifier(f.sender, Repositories{
		Users:    f.users,
		Profiles: f.profiles,
		Programs: &programRepoStub{program: f.program},
		Terms: &termRepoStub{term: &models.ProgramTerm{
			ID: "t1", ProgramID: "p1", Name: "Fall 2026",
			StartDateTime:      timePtr(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)),
			EndDateTime:        timePtr(time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC)),
			ApplicationEndDate: timePtr(time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)),
		}},
		Members:      f.members,
		Applications: &applicationRepoStub{app: &models.Application{ID: "a1", ProgramTermID: "t1", UserID: "mentee"}},
		Tasks:        f.tasks,
	}, NotifierConfig{PublicSiteURL: testPublicSiteURL + "/", SelfServeURL: testSelfServeURL, HRInbox: testHRInbox, ReviewInboxes: []string{testReviewInbox}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return f
}

func (f *fixture) wait(t *testing.T) []Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.n.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return f.sender.sent
}

func TestNotifyMentorInvited(t *testing.T) {
	f := newFixture()
	f.n.NotifyMentorInvited(context.Background(), "p1", "mentor", "inviter", "tok+en/=")

	sent := f.wait(t)
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}
	msg := sent[0]
	if msg.To != "mentor@linuxfoundation.org" || msg.Subject != "You are invited to become a mentor for the Kernel <Dev> mentorship program." {
		t.Fatalf("to/subject = %q / %q", msg.To, msg.Subject)
	}
	inviteURL := testSelfServeURL + "/mentorship/mentor/invites?token=tok%2Ben%2F%3D"
	profileURL := testSelfServeURL + "/mentorship/mentor"
	mustContain(t, "text", msg.Text,
		"Hi Mo Mentor!\n\nAda Admin has added you to be a mentor for the Kernel <Dev> mentorship on LFX Mentorship under the username momentor, a free platform",
		"click Respond to Invitation CTA below to accept the invitation.",
		"If you don't have a mentor profile yet, please click Create Profile CTA below to create your mentor profile",
		"please use this username when logging in to accept the invite",
		"Respond to Invitation: "+inviteURL+"\nCreate Profile: "+profileURL+"\n",
		"The LF Engineering Team",
		"Copyright © "+strconv.Itoa(time.Now().UTC().Year()),
	)
	if strings.Contains(msg.Text, "Hello,") {
		t.Fatalf("text repeats the greeting:\n%s", msg.Text)
	}
	if !strings.Contains(msg.HTML, "Kernel &lt;Dev&gt;") || strings.Contains(msg.HTML, "Kernel <Dev>") {
		t.Fatalf("html did not escape program name:\n%s", msg.HTML)
	}
	mustContain(t, "html", msg.HTML, `href="`+inviteURL+`"`, ">Respond to Invitation</a>", `href="`+profileURL+`"`, ">Create Profile</a>")
}

func TestNotifyMentorInvited_NoLFIDOmitsUsernameNote(t *testing.T) {
	f := newFixture()
	f.users.users["mentor"].LFID = nil
	f.n.NotifyMentorInvited(context.Background(), "p1", "mentor", "inviter", "tok")

	sent := f.wait(t)
	if len(sent) != 1 || strings.Contains(sent[0].Text, "username") || strings.Contains(sent[0].HTML, "username") {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotifyMentorInvited_UnknownInviterStillInvites(t *testing.T) {
	for name, inviterID := range map[string]string{"no inviter": "", "unknown inviter": "missing", "unnamed inviter": "admin1"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.n.NotifyMentorInvited(context.Background(), "p1", "mentor", inviterID, "tok")

			sent := f.wait(t)
			if len(sent) != 1 {
				t.Fatalf("sent = %+v, want the invite", sent)
			}
			mustContain(t, "text", sent[0].Text, "A program administrator has added you to be a mentor")
		})
	}
}

func TestNotifyMentorDeclined(t *testing.T) {
	f := newFixture()
	f.n.NotifyMentorDeclined(context.Background(), "p1", "mentor")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentor@linuxfoundation.org" || sent[0].Subject != "Your mentor request for Kernel <Dev> was declined" {
		t.Fatalf("sent = %+v", sent)
	}
	if !strings.Contains(sent[0].HTML, `href="`+testPublicSiteURL+`/programs/kernel-dev"`) {
		t.Fatalf("html missing program link:\n%s", sent[0].HTML)
	}
}

func TestNotifyAdminMentorDeclined(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}}

	f.n.NotifyAdminMentorDeclined(context.Background(), "p1", "mentor")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "admin1@linuxfoundation.org" {
		t.Fatalf("sent = %+v, want only admin1", sent)
	}
	if sent[0].Subject != "Mo Mentor declined your mentor invitation for Kernel <Dev>" {
		t.Fatalf("subject = %q", sent[0].Subject)
	}
	if !strings.Contains(sent[0].Text, "Mo Mentor declined your invitation to participate as a mentor for the Kernel <Dev> mentorship program.") {
		t.Fatalf("text = %s", sent[0].Text)
	}
	if !strings.Contains(sent[0].HTML, `href="`+testAdminProgramURL+`"`) {
		t.Fatalf("html missing program admin link:\n%s", sent[0].HTML)
	}
}

func TestNotifyAdminMentorAccepted(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}, {UserID: "admin2"}}

	f.n.NotifyAdminMentorAccepted(context.Background(), "p1", "mentor")

	sent := byRecipient(f.wait(t))
	if len(sent) != 2 {
		t.Fatalf("sent = %+v, want admin1 and admin2", sent)
	}
	msg := sent["admin1@linuxfoundation.org"]
	if msg.Subject != "Mo Mentor accepted your invitation to become a mentor for the Kernel <Dev> mentorship project." {
		t.Fatalf("subject = %q", msg.Subject)
	}
	mustContain(t, "text", msg.Text,
		"Hi there!\n\nCongratulations!",
		"Mo Mentor accepted your invitation to participate as a mentor for the Kernel <Dev> mentorship program.",
		"Login to LFX Mentorship at your earliest convenience to view your new mentor's profile.",
		"View Program: "+testAdminProgramURL,
	)
	if !strings.Contains(msg.HTML, `href="`+testAdminProgramURL+`"`) || !strings.Contains(msg.HTML, "Kernel &lt;Dev&gt;") {
		t.Fatalf("html = %s", msg.HTML)
	}
}

func TestNotifyAdminTasksSubmitted(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}, {UserID: "noaddr"}, {UserID: "admin2"}}
	f.sender.failFor = map[string]bool{"admin1@linuxfoundation.org": true}

	f.n.NotifyAdminTasksSubmitted(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "admin2@linuxfoundation.org" {
		t.Fatalf("sent = %+v, want only admin2 (admin1 send failed, noaddr has no email)", sent)
	}
	if sent[0].Subject != "A new mentee submitted application to join Kernel <Dev> mentorship program." {
		t.Fatalf("subject = %q", sent[0].Subject)
	}
	mustContain(t, "text", sent[0].Text,
		"Hi there!",
		"You received an application from Mia Mentee to become a mentee for the Kernel <Dev> project (Fall 2026 term).",
		"View Program: "+testAdminProgramURL,
	)
	want := []models.ProgramMemberFilter{{Limit: pageSize, MemberType: "program_admin", Status: "approved"}}
	if !slices.Equal(f.members.filters, want) {
		t.Fatalf("filters = %+v, want %+v", f.members.filters, want)
	}
}

func TestNotifyAdminTasksSubmitted_PagesThroughAllAdmins(t *testing.T) {
	f := newFixture()
	f.sender.delay = 5 * time.Millisecond
	for i := range pageSize + 1 {
		id := "page-admin-" + strconv.Itoa(i)
		f.users.users[id] = &models.User{ID: id, Email: strPtr(id + "@linuxfoundation.org")}
		f.members.members = append(f.members.members, &models.ProgramMember{UserID: id})
	}

	f.n.NotifyAdminTasksSubmitted(context.Background(), "a1")

	if sent := f.wait(t); len(sent) != pageSize+1 {
		t.Fatalf("sent %d messages, want %d", len(sent), pageSize+1)
	}
	if len(f.members.filters) != 2 || f.members.filters[1].Offset != pageSize {
		t.Fatalf("filters = %+v, want a second page at offset %d", f.members.filters, pageSize)
	}
	// Slow recipients are sent in parallel so they cannot use up the deadline, but never more than the cap at once.
	if f.sender.maxInFlight < 2 || f.sender.maxInFlight > sendConcurrency {
		t.Fatalf("max sends in flight = %d, want between 2 and %d", f.sender.maxInFlight, sendConcurrency)
	}
}

func TestNotifyAdminTasksSubmitted_NoAdmins(t *testing.T) {
	f := newFixture()
	f.n.NotifyAdminTasksSubmitted(context.Background(), "a1")

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotifyMenteeAccepted(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}, {UserID: "mentor"}}

	f.n.NotifyMenteeAccepted(context.Background(), "a1", "full_time")

	sent := byRecipient(f.wait(t))
	if len(sent) != 4 {
		t.Fatalf("sent = %+v, want mentee, both admins, and the HR inbox", sent)
	}

	mentee := sent["mentee@linuxfoundation.org"]
	if mentee.Subject != "Congratulations! You were accepted to Kernel <Dev> mentorship program." {
		t.Fatalf("mentee subject = %q", mentee.Subject)
	}
	mustContain(t, "mentee text", mentee.Text,
		"Hi Mia!\n\nCongratulations!",
		"We are pleased to let you know that you have been accepted as a mentee to the Kernel <Dev> mentorship for the Fall 2026 term on a full-time basis. The program administrator will contact you with the next steps.",
		"Login To LFX Mentorship: "+testSelfServeURL+"/mentorship/mentee/overview",
	)

	admin := sent["admin1@linuxfoundation.org"]
	if admin.Subject != "A new mentee was accepted to Kernel <Dev> mentorship program." {
		t.Fatalf("admin subject = %q", admin.Subject)
	}
	mustContain(t, "admin text", admin.Text,
		"Dear Program Admin and HR Team\n\nMia Mentee has been accepted to the Kernel <Dev> mentorship.",
		"HR team, please generate the acceptance letter and contract paperwork with the following details:",
		"Mentorship Program Name: Kernel <Dev>\nProgram Admin Email: admin1@linuxfoundation.org\nProgram Admin Name: Mo Mentor\nProgram Admin Email: mentor@linuxfoundation.org\nMentee Name: Mia Mentee\nMentee Email: mentee@linuxfoundation.org\n",
		"Mentorship Start Month and Year: September 2026\nMentorship End Month and Year: November 2026\nAttendance Status: full-time\n",
		"Program Admin you will be notified when mentee signs the acceptance letter.",
		"Login To LFX Mentorship: "+testSelfServeURL+"/mentorship\n",
	)
	if strings.Contains(admin.Text, "India") || strings.Contains(admin.HTML, "India") || strings.Contains(admin.Text, "stipend") {
		t.Fatalf("admin copy must not carry the mentee's country:\n%s", admin.Text)
	}

	hr := sent[testHRInbox]
	if hr.Subject != admin.Subject || !strings.Contains(hr.Text, "Dear Program Admin and HR Team") || !strings.Contains(hr.Text, "Program Admin Name: Mo Mentor") {
		t.Fatalf("hr = %+v", hr)
	}
	if !strings.Contains(hr.HTML, "mentor@linuxfoundation.org") {
		t.Fatalf("hr html missing admin contact:\n%s", hr.HTML)
	}
	if !strings.Contains(hr.Text, "Mentee Country of Residence: India (IN)") || !strings.Contains(hr.Text, "NOTE: Please save the mentee country of residence for stipend amount verification.") || !strings.Contains(hr.HTML, "India (IN)") {
		t.Fatalf("hr copy missing country of residence:\n%s", hr.Text)
	}
}

func TestNotifyMenteeAccepted_HRNoticeWithoutCountry(t *testing.T) {
	for name, profiles := range map[string]*profileRepoStub{
		"no mentee profile":  {},
		"lookup fails":       {err: errors.New("db down")},
		"address not object": {address: json.RawMessage(`"Pune, India"`)},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.n.repos.Profiles = profiles
			f.n.NotifyMenteeAccepted(context.Background(), "a1", "full_time")

			hr := byRecipient(f.wait(t))[testHRInbox]
			if hr.To == "" || strings.Contains(hr.Text, "Country of Residence") || strings.Contains(hr.Text, "stipend") {
				t.Fatalf("hr = %+v, want notice without the country row", hr)
			}
		})
	}
}

func TestNotifyMenteeAccepted_HRInboxAlsoAdmin_GetsHRNotice(t *testing.T) {
	f := newFixture()
	f.users.users["hradmin"] = &models.User{ID: "hradmin", Email: strPtr(strings.ToUpper(testHRInbox))}
	f.members.members = []*models.ProgramMember{{UserID: "hradmin"}}

	f.n.NotifyMenteeAccepted(context.Background(), "a1", "full_time")

	var hr []Message
	for _, msg := range f.wait(t) {
		if strings.EqualFold(msg.To, testHRInbox) {
			hr = append(hr, msg)
		}
	}
	if len(hr) != 1 || !strings.Contains(hr[0].Text, "India (IN)") {
		t.Fatalf("hr = %+v, want one HR notice with the country of residence", hr)
	}
}

func TestCountryName(t *testing.T) {
	for in, want := range map[string]string{
		"US":          "United States (US)",
		" gb ":        "United Kingdom (GB)",
		"hk":          "Hong Kong SAR China (HK)",
		"India":       "India",
		"ZZ":          "ZZ",
		"":            "",
		"Deutschland": "Deutschland",
	} {
		if got := countryName(in); got != want {
			t.Errorf("countryName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNotifyMenteeAccepted_WithoutAdmins_MenteeAndHRStillNotified(t *testing.T) {
	f := newFixture()
	f.n.NotifyMenteeAccepted(context.Background(), "a1", "part_time")

	sent := byRecipient(f.wait(t))
	if len(sent) != 2 || !strings.Contains(sent["mentee@linuxfoundation.org"].Text, "on a part-time basis") || !strings.Contains(sent[testHRInbox].Text, "Attendance Status: part-time") {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotify_LookupFailureSendsNothing(t *testing.T) {
	f := newFixture()
	f.n.NotifyMentorInvited(context.Background(), "missing", "mentor", "inviter", "tok")
	f.n.NotifyMentorDeclined(context.Background(), "p1", "noaddr")

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotify_OutlivesRequestContext(t *testing.T) {
	f := newFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f.n.NotifyMentorDeclined(ctx, "p1", "mentor")

	if sent := f.wait(t); len(sent) != 1 {
		t.Fatalf("sent %d messages after request context ended, want 1", len(sent))
	}
}

func TestWait_ReturnsWhenContextEnds(t *testing.T) {
	f := newFixture()
	f.sender.release = make(chan struct{})
	defer close(f.sender.release)
	f.n.NotifyMentorDeclined(context.Background(), "p1", "mentor")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := f.n.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait = %v, want DeadlineExceeded", err)
	}
}

type panickingProgramRepo struct {
	domain.ProgramRepository
}

func (panickingProgramRepo) GetByID(context.Context, string) (*models.Program, error) {
	panic("boom")
}

func TestNotify_PanicIsContained(t *testing.T) {
	f := newFixture()
	f.n.repos.Programs = panickingProgramRepo{}

	f.n.NotifyMentorDeclined(context.Background(), "p1", "mentor")

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotify_DeduplicatesRecipients(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}}
	f.n.hrInbox = "Admin1@linuxfoundation.org"

	f.n.NotifyMenteeAccepted(context.Background(), "a1", "full_time")

	sent := f.wait(t)
	if len(sent) != 2 {
		t.Fatalf("sent %d messages, want mentee plus one for the admin who is also the HR inbox: %+v", len(sent), sent)
	}
}

func TestNotify_AllowedRecipientsSuppressesOthers(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}}
	f.n.allowedRecipients = recipientSet([]string{"Mentee@LinuxFoundation.org"})

	f.n.NotifyMenteeAccepted(context.Background(), "a1", "full_time")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" {
		t.Fatalf("sent = %+v, want only the allowed mentee; the admin and HR inbox are suppressed", sent)
	}
}

func TestRecipientAllowed(t *testing.T) {
	n := &Notifier{allowedRecipients: recipientSet([]string{"Tester@LinuxFoundation.org", "pinned+qa@linuxfoundation.org"})}
	for addr, want := range map[string]bool{
		"tester@linuxfoundation.org":            true,
		"tester+mentee1@linuxfoundation.org":    true,
		"tester@contractor.linuxfoundation.org": false,
		"other@linuxfoundation.org":             false,
		"other+tester@linuxfoundation.org":      false,
		"pinned+qa@linuxfoundation.org":         true,
		"pinned@linuxfoundation.org":            false,
		"pinned+dev@linuxfoundation.org":        false,
	} {
		if got := n.recipientAllowed(addr); got != want {
			t.Errorf("recipientAllowed(%q) = %v, want %v", addr, got, want)
		}
	}
}

// mustContain fails the test unless body contains every want.
func mustContain(t *testing.T, label, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Fatalf("%s missing %q:\n%s", label, w, body)
		}
	}
}

func TestNotifyProgramSubmitted(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}}

	f.n.NotifyProgramSubmitted(context.Background(), "p1", "mentor")

	sent := byRecipient(f.wait(t))
	if len(sent) != 2 {
		t.Fatalf("sent = %+v, want the admin and the review inbox", sent)
	}
	admin := sent["admin1@linuxfoundation.org"]
	if admin.Subject != "Your submission to the LFX Mentorship is pending approval." {
		t.Fatalf("admin subject = %q", admin.Subject)
	}
	mustContain(t, "admin text", admin.Text,
		"Hi there!\n\nThank you for submitting Kernel <Dev> to be a part of our mentorship program on LFX Platform.",
		"You will be notified via email when the review is complete.",
		"View Project: "+testAdminProgramURL,
	)
	review := sent[testReviewInbox]
	if review.Subject != "LFX Mentorship program submission is pending your review." {
		t.Fatalf("review subject = %q", review.Subject)
	}
	reviewURL := testSelfServeURL + "/mentorship/program-review/p1?decision="
	mustContain(t, "review text", review.Text, "Hi LF Mentorship team!", "submitted for review by Mo Mentor (mentor@linuxfoundation.org).", "View Submission: "+testPublicSiteURL+"/programs/kernel-dev\nApprove: "+reviewURL+"approve\nReject: "+reviewURL+"reject\n")
	mustContain(t, "review html", review.HTML, `href="`+testPublicSiteURL+`/programs/kernel-dev"`, ">View Submission</a>", `href="`+reviewURL+`approve"`, ">Approve</a>", `href="`+reviewURL+`reject"`, ">Reject</a>", "Kernel &lt;Dev&gt;")
}

func TestNotifyProgramSubmitted_EveryReviewInboxGetsACopy(t *testing.T) {
	f := newFixture()
	f.n.reviewInboxes = []string{testReviewInbox, "second-review@linuxfoundation.org"}

	f.n.NotifyProgramSubmitted(context.Background(), "p1", "")

	sent := byRecipient(f.wait(t))
	if len(sent) != 2 {
		t.Fatalf("sent = %+v, want one notice per review inbox", sent)
	}
	for _, to := range f.n.reviewInboxes {
		if sent[to].Subject != "LFX Mentorship program submission is pending your review." {
			t.Fatalf("%s subject = %q", to, sent[to].Subject)
		}
	}
}

func TestNotifyProgramSubmitted_UnnamedSubmitterNamedByLFID(t *testing.T) {
	f := newFixture()
	f.users.users["unnamed"] = &models.User{ID: "unnamed", LFID: strPtr("quiet-admin"), Email: strPtr("quiet@linuxfoundation.org")}

	f.n.NotifyProgramSubmitted(context.Background(), "p1", "unnamed")

	review := byRecipient(f.wait(t))[testReviewInbox]
	mustContain(t, "review text", review.Text, "submitted for review by quiet-admin (quiet@linuxfoundation.org).")
}

func TestNotifyProgramSubmitted_UnknownSubmitterStillNotifiesReviewers(t *testing.T) {
	for name, submitter := range map[string]string{"no actor": "", "lookup fails": "missing"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.n.NotifyProgramSubmitted(context.Background(), "p1", submitter)

			sent := f.wait(t)
			if len(sent) != 1 || sent[0].To != testReviewInbox {
				t.Fatalf("sent = %+v, want only the review inbox", sent)
			}
			mustContain(t, "review text", sent[0].Text, "The Kernel <Dev> mentorship program was submitted for review.")
		})
	}
}

func TestNotifyProgramDecided(t *testing.T) {
	for _, tc := range []struct {
		status  models.ProgramStatus
		subject string
		want    []string
	}{
		{models.ProgramStatusPublished, "Your submission for the LFX Mentorship has been approved.", []string{"Hi there!\n\nCongratulations!", "Kernel <Dev> has been approved to participate in LFX Mentorship.\n", "You can manage your project by selecting View Project below", "View Project: " + testAdminProgramURL}},
		{models.ProgramStatusRejected, "Your program submission to the LFX Mentorship was rejected.", []string{"Hi there\n\nRegrettably, Kernel <Dev> has not been approved", "Please contact our support team", supportDeskURL, "View Submission: " + testAdminProgramURL}},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			f := newFixture()
			f.members.members = []*models.ProgramMember{{UserID: "admin1"}, {UserID: "admin2"}}

			f.n.NotifyProgramDecided(context.Background(), "p1", tc.status)

			sent := byRecipient(f.wait(t))
			if len(sent) != 2 {
				t.Fatalf("sent = %+v, want both admins", sent)
			}
			msg := sent["admin2@linuxfoundation.org"]
			if msg.Subject != tc.subject {
				t.Fatalf("subject = %q, want %q", msg.Subject, tc.subject)
			}
			mustContain(t, "text", msg.Text, tc.want...)
		})
	}
}

func TestNotifyProgramUpdated(t *testing.T) {
	f := newFixture()
	f.n.NotifyProgramUpdated(context.Background(), "p1", "mentor")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentor@linuxfoundation.org" || sent[0].Subject != "Your update was successful." {
		t.Fatalf("sent = %+v, want only the editor", sent)
	}
	mustContain(t, "text", sent[0].Text,
		"Hi Mo Mentor!\n\nJust confirming that your updates to Kernel <Dev> have been saved.",
		"View Program: "+testAdminProgramURL,
	)
}

func TestNotifyProfileCreated(t *testing.T) {
	for name, tc := range map[string]struct {
		profileType models.UserProfileType
		subject     string
		want        []string
	}{
		"mentor": {
			profileType: models.UserProfileTypeMentor,
			subject:     "Your LFX Mentorship mentor profile was successfully created.",
			want: []string{
				"Hi Mia!\n\nThank you for taking the first step to become a mentor on LFX Mentorship.",
				"If you haven't requested to be added to a program yet, please select LFX Mentorship programs you would like to join as a mentor",
				"View Profile: " + testSelfServeURL + "/mentorship/mentor/profile",
			},
		},
		"mentee": {
			profileType: models.UserProfileTypeMentee,
			subject:     "Your LFX Mentorship mentee profile was successfully created.",
			want: []string{
				"Hi Mia!\n\nHurray Step 1 is Complete! Thank you for taking the first step to become a mentee on LFX Mentorship!",
				"- Step 1: You are registered as a Mentee - Complete!\n- Step 2: Please view and apply to the Mentorship Programs based on your skillsets\n",
				"Show me the Mentorship Projects: " + testPublicSiteURL + "/programs",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.n.NotifyProfileCreated(context.Background(), "mentee", tc.profileType)

			sent := f.wait(t)
			if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != tc.subject {
				t.Fatalf("sent = %+v", sent)
			}
			mustContain(t, "text", sent[0].Text, tc.want...)
		})
	}
}

func TestNotifyProfileCreated_UnknownTypeSendsNothing(t *testing.T) {
	f := newFixture()
	f.n.NotifyProfileCreated(context.Background(), "mentee", models.UserProfileType("employer"))

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotifyProgramDecided_OtherStatusSendsNothing(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}}

	f.n.NotifyProgramDecided(context.Background(), "p1", models.ProgramStatusHidden)

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotifyAdminMentorRequested_PrefersMemberEmail(t *testing.T) {
	for name, tc := range map[string]struct {
		row  *models.ProgramMember
		err  error
		want string
	}{
		"member row email":         {row: &models.ProgramMember{Email: strPtr(" current@linuxfoundation.org ")}, want: "current@linuxfoundation.org"},
		"member row without email": {row: &models.ProgramMember{}, want: "mentor@linuxfoundation.org"},
		"no member row":            {want: "mentor@linuxfoundation.org"},
		"lookup fails":             {err: errors.New("db down"), want: "mentor@linuxfoundation.org"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.members.members = []*models.ProgramMember{{UserID: "admin1"}}
			f.members.mentorRow, f.members.mentorErr = tc.row, tc.err

			f.n.NotifyAdminMentorRequested(context.Background(), "p1", "mentor")

			sent := f.wait(t)
			if len(sent) != 1 {
				t.Fatalf("sent = %+v", sent)
			}
			mustContain(t, "text", sent[0].Text, "contact them at "+tc.want+"\n")
		})
	}
}

func TestNotifyAdminMentorRequestedAndWithdrew(t *testing.T) {
	for name, tc := range map[string]struct {
		notify  func(*Notifier)
		subject string
		want    []string
	}{
		"requested": {
			notify:  func(n *Notifier) { n.NotifyAdminMentorRequested(context.Background(), "p1", "mentor") },
			subject: "A new mentor submitted request to join Kernel <Dev> mentorship program.",
			want: []string{
				"Mo Mentor\nRequested to become a mentor\n\nMo Mentor submitted a request to join the Kernel <Dev> mentorship project as a mentor.",
				"Please select the CTA below to review the request on your program's Mentors tab, then contact them at mentor@linuxfoundation.org",
				"If you believe this mentor is not a good fit for your project, please contact them as well and let them know.",
				"Add to Program: " + testAdminProgramURL,
			},
		},
		"withdrew": {
			notify:  func(n *Notifier) { n.NotifyAdminMentorWithdrew(context.Background(), "p1", "mentor") },
			subject: "Mo Mentor withdrew a request to join your program as a mentor.",
			want:    []string{"Hi there,\n\nRegrettably, Mo Mentor withdrew the request to participate as a mentor for the Kernel <Dev> mentorship program.", "View Program: " + testAdminProgramURL},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.members.members = []*models.ProgramMember{{UserID: "admin1"}}

			tc.notify(f.n)

			sent := f.wait(t)
			if len(sent) != 1 || sent[0].To != "admin1@linuxfoundation.org" || sent[0].Subject != tc.subject {
				t.Fatalf("sent = %+v", sent)
			}
			mustContain(t, "text", sent[0].Text, tc.want...)
		})
	}
}

func TestNotifyMenteeApplicationReceived(t *testing.T) {
	prerequisite, other := models.TaskCategoryPrerequisite, models.TaskCategoryNonPrerequisite
	f := newFixture()
	f.tasks.tasks = []*models.Task{
		{ID: "t1", Name: strPtr("Write an <intro>"), Category: &prerequisite, DueDate: strPtr("2026-08-01")},
		{ID: "t2", Name: strPtr("Unrelated"), Category: &other},
		{ID: "t3", Name: strPtr("  "), Category: &prerequisite},
		{ID: "t4", Name: strPtr("Fix a bug"), Category: &prerequisite},
	}

	f.n.NotifyMenteeApplicationReceived(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "Your application to Kernel <Dev> mentorship program was received." {
		t.Fatalf("sent = %+v", sent)
	}
	mustContain(t, "text", sent[0].Text,
		"Hi Mia!",
		"you need to complete a set of prerequisite tasks and submit them for review.",
		// A task without its own due date is due when the term's applications close.
		"Prerequisite tasks:\n\nTask Name: Write an <intro>\nDue Date: August 1, 2026\n\nTask Name: Fix a bug\nDue Date: August 15, 2026\n\n",
		"Please login to LFX Mentorship at your earliest convenience to complete the tasks or select the link below.",
		"View Prerequisite Tasks: "+testSelfServeURL+"/mentorship/mentee/tasks",
	)
	if strings.Contains(sent[0].Text, "Unrelated") || strings.Contains(sent[0].Text, "Task Created") {
		t.Fatalf("text lists a task that is not a prerequisite, or a creation date:\n%s", sent[0].Text)
	}
	mustContain(t, "html", sent[0].HTML, "Write an &lt;intro&gt;</strong>", "August 1, 2026", ">View Prerequisite Tasks</a>")
}

func TestNotifyMenteeApplicationReceived_NoDueDateAnywhere(t *testing.T) {
	prerequisite := models.TaskCategoryPrerequisite
	f := newFixture()
	f.n.repos.Terms = &termRepoStub{term: &models.ProgramTerm{ID: "t1", ProgramID: "p1", Name: "Fall 2026"}}
	f.tasks.tasks = []*models.Task{{ID: "t1", Name: strPtr("Fix a bug"), Category: &prerequisite}}

	f.n.NotifyMenteeApplicationReceived(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 || !strings.Contains(sent[0].Text, "Task Name: Fix a bug\n") || strings.Contains(sent[0].Text, "Due Date") {
		t.Fatalf("sent = %+v, want the task without a due date row", sent)
	}
}

func TestNotifyMenteePrerequisiteReminder(t *testing.T) {
	prerequisite, other := models.TaskCategoryPrerequisite, models.TaskCategoryNonPrerequisite
	f := newFixture()
	f.tasks.tasks = []*models.Task{
		{ID: "t1", Name: strPtr("Write an intro"), Category: &prerequisite, Status: models.TaskStatusIncomplete},
		{ID: "t2", Name: strPtr("Fix a bug"), Category: &prerequisite, Status: models.TaskStatusInProgress, DueDate: strPtr("2026-08-01")},
		{ID: "t3", Name: strPtr("Already sent"), Category: &prerequisite, Status: models.TaskStatusSubmitted},
		{ID: "t4", Name: strPtr("Already done"), Category: &prerequisite, Status: models.TaskStatusComplete},
		{ID: "t5", Name: strPtr("Unrelated"), Category: &other, Status: models.TaskStatusIncomplete},
	}

	f.n.NotifyMenteePrerequisiteReminder(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "Complete your prerequisite tasks for Kernel <Dev>" {
		t.Fatalf("sent = %+v", sent)
	}
	mustContain(t, "text", sent[0].Text,
		"Hi Mia!\n\nThank you for your interest in becoming a mentee on LFX Mentorship!",
		"This is a reminder that your application for the Kernel <Dev> mentorship program (Fall 2026) still has incomplete prerequisite tasks.",
		"Due date: August 15, 2026\n",
		"Incomplete prerequisite tasks:\n\nTask Name: Write an intro\nDue Date: August 15, 2026\n\nTask Name: Fix a bug\nDue Date: August 1, 2026\n\n",
		"View Prerequisite Tasks: "+testSelfServeURL+"/mentorship/mentee/tasks",
	)
	for _, notWant := range []string{"Already sent", "Already done", "Unrelated"} {
		if strings.Contains(sent[0].Text, notWant) {
			t.Fatalf("reminder lists %q:\n%s", notWant, sent[0].Text)
		}
	}
	mustContain(t, "html", sent[0].HTML, "<strong>(Fall 2026)</strong>", "<strong>Due date: August 15, 2026</strong>", ">View Prerequisite Tasks</a>")
}

func TestNotifyMenteePrerequisiteReminder_NothingPendingSendsNothing(t *testing.T) {
	prerequisite := models.TaskCategoryPrerequisite
	f := newFixture()
	f.tasks.tasks = []*models.Task{{ID: "t1", Name: strPtr("Already sent"), Category: &prerequisite, Status: models.TaskStatusSubmitted}}

	f.n.NotifyMenteePrerequisiteReminder(context.Background(), "a1")

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v, want no reminder", sent)
	}
}

func TestNotifyMenteeApplicationReceived_PagesThroughAllTasks(t *testing.T) {
	prerequisite := models.TaskCategoryPrerequisite
	f := newFixture()
	for i := range pageSize + 1 {
		f.tasks.tasks = append(f.tasks.tasks, &models.Task{ID: "t" + strconv.Itoa(i), Name: strPtr("Task " + strconv.Itoa(i)), Category: &prerequisite})
	}

	f.n.NotifyMenteeApplicationReceived(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 {
		t.Fatalf("sent = %+v", sent)
	}
	if got := strings.Count(sent[0].Text, "Task Name: Task "); got != pageSize+1 {
		t.Fatalf("listed %d prerequisite tasks, want %d", got, pageSize+1)
	}
}

func TestNotifyMenteeApplicationReceived_NoTasks(t *testing.T) {
	f := newFixture()
	f.n.NotifyMenteeApplicationReceived(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 {
		t.Fatalf("sent = %+v", sent)
	}
	mustContain(t, "text", sent[0].Text, "Your mentorship application will be reviewed, and you will be notified", "View Application: "+testSelfServeURL+"/mentorship/mentee/overview")
	if strings.Contains(sent[0].Text, "Prerequisite tasks") || strings.Contains(sent[0].Text, "prerequisite tasks") {
		t.Fatalf("text lists prerequisite tasks that do not exist:\n%s", sent[0].Text)
	}
}

func TestNotifyMenteeApplicationReceived_TaskLookupFailureSendsNothing(t *testing.T) {
	f := newFixture()
	f.tasks.err = errors.New("db down")
	f.n.NotifyMenteeApplicationReceived(context.Background(), "a1")

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotifyMenteeDeclined(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}}

	f.n.NotifyMenteeDeclined(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "Your application to Kernel <Dev> mentorship wasn't accepted." {
		t.Fatalf("sent = %+v, want only the mentee", sent)
	}
	mustContain(t, "text", sent[0].Text,
		"Hi Mia\n\nRegrettably, your application to be a mentee for Kernel <Dev> mentorship has not been accepted.",
		"Please reach out to program administrator with any questions",
		"mentorship@linuxfoundation.org",
		"Login To Your Account: "+testSelfServeURL+"/mentorship/mentee/overview",
	)
}

func TestNotifyMenteeWithdrawn(t *testing.T) {
	f := newFixture()
	f.members.members = []*models.ProgramMember{{UserID: "admin1"}}

	f.n.NotifyMenteeWithdrawn(context.Background(), "a1")

	sent := byRecipient(f.wait(t))
	if len(sent) != 2 {
		t.Fatalf("sent = %+v, want the mentee and the admin", sent)
	}
	mentee := sent["mentee@linuxfoundation.org"]
	if mentee.Subject != "Your application to Kernel <Dev> mentorship program was withdrawn." {
		t.Fatalf("mentee subject = %q", mentee.Subject)
	}
	mustContain(t, "mentee text", mentee.Text,
		"Hi Mia,\n\nWe just want to confirm that your application to be a mentee for Kernel <Dev> mentorship program has been withdrawn.",
		"Login To Your Account: "+testSelfServeURL+"/mentorship/mentee/overview",
	)
	admin := sent["admin1@linuxfoundation.org"]
	if admin.Subject != "A mentee application to Kernel <Dev> mentorship was withdrawn." {
		t.Fatalf("admin subject = %q", admin.Subject)
	}
	mustContain(t, "admin text", admin.Text,
		"Hi there,\n\nWe wanted to let you know that the application submitted by Mia Mentee for the Kernel <Dev> mentorship program (Fall 2026 term) has been withdrawn.",
		"View Program: "+testAdminProgramURL,
	)
}

func TestNotifyMenteeGraduated(t *testing.T) {
	f := newFixture()
	f.members.mentors = []*models.ProgramMentorManagementRow{
		{UserID: "mentor", Name: strPtr("Mo Mentor"), Username: strPtr("momentor")},
		{UserID: "mentor2", Name: strPtr(" Ana Guide ")},
		{UserID: "unnamed", Name: strPtr(" "), Username: strPtr("quiet-mentor")},
		{UserID: "anonymous"},
	}

	f.n.NotifyMenteeGraduated(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "Congratulations! You have graduated from Kernel <Dev> mentorship program." {
		t.Fatalf("sent = %+v, want only the mentee", sent)
	}
	msg := sent[0]
	mustContain(t, "text", msg.Text,
		time.Now().UTC().Format(dateLayout)+"\n\nTo Whom It May Concern:",
		"certify that Mia Mentee successfully completed the Kernel <Dev> Program at The Linux Foundation from September 1, 2026 - November 30, 2026.",
		"Mia Mentee worked on the Kernel <Dev>, mentored by Mo Mentor, Ana Guide, quiet-mentor. A brief description of the Project they worked on is given below:",
		"Work on <drivers>.",
		"Sincerely,\nThe Linux Foundation Mentorship Team,\nThe Linux Foundation",
	)
	mustContain(t, "html", msg.HTML, "Work on &lt;drivers&gt;.")
	for _, body := range []string{msg.Text, msg.HTML} {
		if strings.Contains(body, "Hi ") || strings.Contains(body, "Thank you,") {
			t.Fatalf("certificate letter carries the notice greeting or sign-off:\n%s", body)
		}
	}
	want := []models.ProgramMemberFilter{{Limit: mentorPageSize, Status: "approved"}}
	if !slices.Equal(f.members.mentorFilters, want) {
		t.Fatalf("mentor filters = %+v, want %+v", f.members.mentorFilters, want)
	}
}

func TestNotifyMenteeGraduated_PagesThroughAllMentors(t *testing.T) {
	f := newFixture()
	for i := range mentorPageSize + 1 {
		f.members.mentors = append(f.members.mentors, &models.ProgramMentorManagementRow{Username: strPtr("mentor-" + strconv.Itoa(i))})
	}

	f.n.NotifyMenteeGraduated(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 {
		t.Fatalf("sent = %+v", sent)
	}
	mustContain(t, "text", sent[0].Text, "mentor-0, ", ", mentor-"+strconv.Itoa(mentorPageSize)+".")
	if len(f.members.mentorFilters) != 2 || f.members.mentorFilters[1].Offset != mentorPageSize {
		t.Fatalf("mentor filters = %+v, want a second page at offset %d", f.members.mentorFilters, mentorPageSize)
	}
}

func TestNotifyMenteeGraduated_WithoutMentorsOrDescription(t *testing.T) {
	f := newFixture()
	f.program.Description = nil
	f.n.NotifyMenteeGraduated(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 {
		t.Fatalf("sent = %+v", sent)
	}
	mustContain(t, "text", sent[0].Text, "Mia Mentee worked on the Kernel <Dev>.\n\nMia Mentee learned")
	if strings.Contains(sent[0].Text, "mentored by") || strings.Contains(sent[0].Text, "brief description") {
		t.Fatalf("text = %s", sent[0].Text)
	}
}

func TestNotifyMenteeTaskAssigned(t *testing.T) {
	appID := "a1"
	created := time.Date(2026, time.October, 5, 23, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		due     *string
		want    string
		notWant string
	}{
		"with due date":    {due: strPtr("2026-10-20"), want: "Due Date: October 20, 2026\n"},
		"without due date": {notWant: "Due Date"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.tasks.tasks = []*models.Task{{ID: "task-1", ApplicationID: &appID, AssigneeID: "mentee", Name: strPtr("Ship <it>"), DueDate: tc.due, CreatedOn: created}}

			f.n.NotifyMenteeTaskAssigned(context.Background(), "task-1")

			sent := f.wait(t)
			if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "You were assigned a new task for Kernel <Dev> mentorship program." {
				t.Fatalf("sent = %+v", sent)
			}
			mustContain(t, "text", sent[0].Text,
				"Hi Mia!",
				"Please login to the LFX Mentorship at your earliest convenience to review your tasks and get started!",
				"Task Name: Ship <it>\nTask Created on: October 5, 2026\n",
				"View Tasks: "+testSelfServeURL+"/mentorship/mentee/tasks",
			)
			mustContain(t, "html", sent[0].HTML, "Ship &lt;it&gt;")
			if tc.want != "" {
				mustContain(t, "text", sent[0].Text, tc.want)
			}
			if tc.notWant != "" && (strings.Contains(sent[0].Text, tc.notWant) || strings.Contains(sent[0].HTML, tc.notWant)) {
				t.Fatalf("message mentions %q:\n%s", tc.notWant, sent[0].Text)
			}
		})
	}
}

func TestNotifyMenteeTaskAssigned_TaskWithoutApplicationSendsNothing(t *testing.T) {
	f := newFixture()
	f.tasks.tasks = []*models.Task{{ID: "task-1", AssigneeID: "mentee"}}
	f.n.NotifyMenteeTaskAssigned(context.Background(), "task-1")
	f.n.NotifyMenteeTaskAssigned(context.Background(), "missing")

	if sent := f.wait(t); len(sent) != 0 {
		t.Fatalf("sent = %+v", sent)
	}
}

// countingProgramRepo records how many notifications resolve their program at once.
type countingProgramRepo struct {
	domain.ProgramRepository
	program  *models.Program
	mu       sync.Mutex
	inFlight int
	max      int
}

func (r *countingProgramRepo) GetByID(context.Context, string) (*models.Program, error) {
	r.mu.Lock()
	r.inFlight++
	r.max = max(r.max, r.inFlight)
	r.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	r.mu.Lock()
	r.inFlight--
	r.mu.Unlock()
	return r.program, nil
}

func TestNotify_BoundsConcurrentDispatches(t *testing.T) {
	f := newFixture()
	programs := &countingProgramRepo{program: f.program}
	f.n.repos.Programs = programs
	const notifications = 3 * maxConcurrentDispatches

	// A bulk decline fires one notification per mentee; they queue for a slot rather than all running at once.
	for range notifications {
		f.n.NotifyMentorDeclined(context.Background(), "p1", "mentor")
	}

	if sent := f.wait(t); len(sent) != notifications {
		t.Fatalf("sent %d messages, want %d", len(sent), notifications)
	}
	if programs.max < 2 || programs.max > maxConcurrentDispatches {
		t.Fatalf("max notifications in flight = %d, want between 2 and %d", programs.max, maxConcurrentDispatches)
	}
}

func TestDueDate(t *testing.T) {
	for in, want := range map[string]string{"2026-02-05": "February 5, 2026", "next week": "next week", "": ""} {
		if got := dueDate(&in); got != want {
			t.Errorf("dueDate(%q) = %q, want %q", in, got, want)
		}
	}
	if got := dueDate(nil); got != "" {
		t.Errorf("dueDate(nil) = %q", got)
	}
}
