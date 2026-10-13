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
			"mentor": {ID: "mentor", Email: strPtr("mentor@linuxfoundation.org"), Name: strPtr("Mo Mentor"), LFID: strPtr("momentor")},
			"mentee": {ID: "mentee", Email: strPtr("mentee@linuxfoundation.org"), GivenName: strPtr("Mia"), FamilyName: strPtr("Mentee")},
			"admin1": {ID: "admin1", Email: strPtr("admin1@linuxfoundation.org")},
			"admin2": {ID: "admin2", Email: strPtr("admin2@linuxfoundation.org")},
			"noaddr": {ID: "noaddr"},
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
			StartDateTime: timePtr(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)),
			EndDateTime:   timePtr(time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC)),
		}},
		Members:      f.members,
		Applications: &applicationRepoStub{app: &models.Application{ID: "a1", ProgramTermID: "t1", UserID: "mentee"}},
		Tasks:        f.tasks,
	}, NotifierConfig{PublicSiteURL: testPublicSiteURL + "/", SelfServeURL: testSelfServeURL, HRInbox: testHRInbox, ReviewInbox: testReviewInbox}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	f.n.NotifyMentorInvited(context.Background(), "p1", "mentor", "tok+en/=")

	sent := f.wait(t)
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}
	msg := sent[0]
	if msg.To != "mentor@linuxfoundation.org" || msg.Subject != "You've been invited to mentor for Kernel <Dev>" {
		t.Fatalf("to/subject = %q / %q", msg.To, msg.Subject)
	}
	inviteURL := testSelfServeURL + "/mentorship/mentor/invites?token=tok%2Ben%2F%3D"
	for _, want := range []string{"Hi Mo Mentor!", inviteURL, "log in with the username momentor", "Copyright © " + strconv.Itoa(time.Now().UTC().Year())} {
		if !strings.Contains(msg.Text, want) {
			t.Fatalf("text missing %q:\n%s", want, msg.Text)
		}
	}
	if !strings.Contains(msg.HTML, "Kernel &lt;Dev&gt;") || strings.Contains(msg.HTML, "Kernel <Dev>") {
		t.Fatalf("html did not escape program name:\n%s", msg.HTML)
	}
	if !strings.Contains(msg.HTML, `href="`+inviteURL+`"`) || !strings.Contains(msg.HTML, ">Respond to Invitation</a>") {
		t.Fatalf("html missing invite button:\n%s", msg.HTML)
	}
}

func TestNotifyMentorInvited_NoLFIDOmitsUsernameNote(t *testing.T) {
	f := newFixture()
	f.users.users["mentor"].LFID = nil
	f.n.NotifyMentorInvited(context.Background(), "p1", "mentor", "tok")

	sent := f.wait(t)
	if len(sent) != 1 || strings.Contains(sent[0].Text, "username") || strings.Contains(sent[0].HTML, "username") {
		t.Fatalf("sent = %+v", sent)
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
	if !strings.Contains(sent[0].HTML, `href="`+testSelfServeURL+`/mentorship"`) {
		t.Fatalf("html missing Self Serve link:\n%s", sent[0].HTML)
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
	if msg.Subject != "Mo Mentor accepted your mentor invitation for Kernel <Dev>" {
		t.Fatalf("subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Text, "Mo Mentor accepted your invitation to participate as a mentor for the Kernel <Dev> mentorship program.") {
		t.Fatalf("text = %s", msg.Text)
	}
	if !strings.Contains(msg.HTML, `href="`+testSelfServeURL+`/mentorship"`) || !strings.Contains(msg.HTML, "Kernel &lt;Dev&gt;") {
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
	if sent[0].Subject != "New mentee application to review for Kernel <Dev>" {
		t.Fatalf("subject = %q", sent[0].Subject)
	}
	if !strings.Contains(sent[0].Text, "Hi there!") || !strings.Contains(sent[0].Text, "application from Mia Mentee to become a mentee for the Kernel <Dev> program (Fall 2026 term).") {
		t.Fatalf("text = %s", sent[0].Text)
	}
	want := []models.ProgramMemberFilter{{Limit: pageSize, MemberType: "program_admin", Status: "active"}}
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
	if mentee.Subject != "Congratulations! You've been accepted to Kernel <Dev>" {
		t.Fatalf("mentee subject = %q", mentee.Subject)
	}
	if !strings.Contains(mentee.Text, "Hi Mia!") || !strings.Contains(mentee.Text, "accepted as a mentee to the Kernel <Dev> mentorship for the Fall 2026 term on a full-time basis.") {
		t.Fatalf("mentee text = %s", mentee.Text)
	}

	admin := sent["admin1@linuxfoundation.org"]
	if admin.Subject != "Mia Mentee has been accepted to Kernel <Dev>" {
		t.Fatalf("admin subject = %q", admin.Subject)
	}
	for _, want := range []string{
		"Program Admin: admin1@linuxfoundation.org\n",
		"Program Admin: Mo Mentor (mentor@linuxfoundation.org)",
		"Mentee Email: mentee@linuxfoundation.org",
		"Mentorship Period: September 2026 – November 2026",
		"Attendance Status: full-time",
	} {
		if !strings.Contains(admin.Text, want) {
			t.Fatalf("admin text missing %q:\n%s", want, admin.Text)
		}
	}
	if strings.Contains(admin.Text, "acceptance letter") {
		t.Fatalf("admin copy must not carry the HR request:\n%s", admin.Text)
	}
	if strings.Contains(admin.Text, "India") || strings.Contains(admin.HTML, "India") {
		t.Fatalf("admin copy must not carry the mentee's country:\n%s", admin.Text)
	}

	hr := sent[testHRInbox]
	if hr.Subject != admin.Subject || !strings.Contains(hr.Text, "Hi HR team!") || !strings.Contains(hr.Text, "acceptance letter") || !strings.Contains(hr.Text, "Program Admin: Mo Mentor (mentor@linuxfoundation.org)") {
		t.Fatalf("hr = %+v", hr)
	}
	if !strings.Contains(hr.HTML, "Mo Mentor (mentor@linuxfoundation.org)") {
		t.Fatalf("hr html missing admin contact:\n%s", hr.HTML)
	}
	if !strings.Contains(hr.Text, "Mentee Country of Residence: India (IN)") || !strings.Contains(hr.Text, "stipend amount verification") || !strings.Contains(hr.HTML, "India (IN)") {
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
	if len(hr) != 1 || !strings.Contains(hr[0].Text, "Hi HR team!") || !strings.Contains(hr[0].Text, "India (IN)") {
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
	if len(sent) != 2 || !strings.Contains(sent["mentee@linuxfoundation.org"].Text, "part-time") || sent[testHRInbox].To == "" {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotify_LookupFailureSendsNothing(t *testing.T) {
	f := newFixture()
	f.n.NotifyMentorInvited(context.Background(), "missing", "mentor", "tok")
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
	if admin.Subject != "We received your submission of Kernel <Dev>" {
		t.Fatalf("admin subject = %q", admin.Subject)
	}
	mustContain(t, "admin text", admin.Text, "Thank you for submitting Kernel <Dev>", "up to 5 business days", testSelfServeURL+"/mentorship")
	review := sent[testReviewInbox]
	if review.Subject != "Mentorship program submitted for review: Kernel <Dev>" {
		t.Fatalf("review subject = %q", review.Subject)
	}
	mustContain(t, "review text", review.Text, "Hi LF Mentorship team!", "submitted for review by Mo Mentor (mentor@linuxfoundation.org).", "Review in Self Serve: "+testSelfServeURL+"/mentorship")
	mustContain(t, "review html", review.HTML, ">Review in Self Serve</a>", "Kernel &lt;Dev&gt;")
	if strings.Contains(review.HTML, "Approve") || strings.Contains(review.HTML, "Reject") {
		t.Fatalf("review notice must not carry approve or reject links:\n%s", review.HTML)
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
		{models.ProgramStatusPublished, "Kernel <Dev> has been approved", []string{"approved to participate in LFX Mentorship and is now live", "View program: " + testPublicSiteURL + "/programs/kernel-dev"}},
		{models.ProgramStatusRejected, "Kernel <Dev> was not approved", []string{"has not been approved", "Please contact our support team", supportDeskURL, "Manage your program: " + testSelfServeURL + "/mentorship"}},
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
			mustContain(t, "text", sent[0].Text, "contact them at "+tc.want+".")
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
			subject: "Mo Mentor requested to mentor for Kernel <Dev>",
			want:    []string{"Mo Mentor requested to join the Kernel <Dev> mentorship program as a mentor.", "contact them at mentor@linuxfoundation.org", "Review the request: " + testSelfServeURL + "/mentorship"},
		},
		"withdrew": {
			notify:  func(n *Notifier) { n.NotifyAdminMentorWithdrew(context.Background(), "p1", "mentor") },
			subject: "Mo Mentor withdrew their mentor request for Kernel <Dev>",
			want:    []string{"Mo Mentor withdrew the request to participate as a mentor for the Kernel <Dev> mentorship program.", testSelfServeURL + "/mentorship"},
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
		{ID: "t1", Name: strPtr("Write an <intro>"), Category: &prerequisite},
		{ID: "t2", Name: strPtr("Unrelated"), Category: &other},
		{ID: "t3", Name: strPtr("  "), Category: &prerequisite},
		{ID: "t4", Name: strPtr("Fix a bug"), Category: &prerequisite},
	}

	f.n.NotifyMenteeApplicationReceived(context.Background(), "a1")

	sent := f.wait(t)
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "We received your application to Kernel <Dev>" {
		t.Fatalf("sent = %+v", sent)
	}
	mustContain(t, "text", sent[0].Text, "Hi Mia!", "Prerequisite tasks:\n- Write an <intro>\n- Fix a bug\n", "View your prerequisite tasks: "+testSelfServeURL+"/mentorship")
	if strings.Contains(sent[0].Text, "Unrelated") {
		t.Fatalf("text lists a task that is not a prerequisite:\n%s", sent[0].Text)
	}
	mustContain(t, "html", sent[0].HTML, "<li>Write an &lt;intro&gt;</li>", ">View Prerequisite Tasks</a>")
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
	if got := strings.Count(sent[0].Text, "\n- Task "); got != pageSize+1 {
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
	mustContain(t, "text", sent[0].Text, "It will be reviewed, and you will be notified", "View your application: ")
	if strings.Contains(sent[0].Text, "Prerequisite tasks") {
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
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "An update on your application to Kernel <Dev>" {
		t.Fatalf("sent = %+v, want only the mentee", sent)
	}
	mustContain(t, "text", sent[0].Text, "has not been accepted", "mentorship@linuxfoundation.org", "Log in to LFX Mentorship: "+testSelfServeURL+"/mentorship")
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
	if mentee.Subject != "Your application to Kernel <Dev> has been withdrawn" {
		t.Fatalf("mentee subject = %q", mentee.Subject)
	}
	mustContain(t, "mentee text", mentee.Text, "Hi Mia!", "has been withdrawn", "View program: "+testPublicSiteURL+"/programs/kernel-dev")
	admin := sent["admin1@linuxfoundation.org"]
	if admin.Subject != "Mia Mentee's application to Kernel <Dev> was withdrawn" {
		t.Fatalf("admin subject = %q", admin.Subject)
	}
	mustContain(t, "admin text", admin.Text, "the application submitted by Mia Mentee for the Kernel <Dev> mentorship program (Fall 2026 term) has been withdrawn.", testSelfServeURL+"/mentorship")
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
	if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "Your LFX Mentorship certificate of completion for Kernel <Dev>" {
		t.Fatalf("sent = %+v, want only the mentee", sent)
	}
	msg := sent[0]
	mustContain(t, "text", msg.Text,
		time.Now().UTC().Format(dateLayout)+"\n\nTo Whom It May Concern:",
		"certify that Mia Mentee successfully completed the Kernel <Dev> mentorship program at The Linux Foundation from September 1, 2026 – November 30, 2026.",
		"mentored by Mo Mentor, Ana Guide, quiet-mentor.",
		"Work on <drivers>.",
		"Sincerely,\nThe Linux Foundation Mentorship Team",
	)
	mustContain(t, "html", msg.HTML, "Work on &lt;drivers&gt;.")
	for _, body := range []string{msg.Text, msg.HTML} {
		if strings.Contains(body, "Hi ") || strings.Contains(body, "Thank you,") {
			t.Fatalf("certificate letter carries the notice greeting or sign-off:\n%s", body)
		}
	}
	want := []models.ProgramMemberFilter{{Limit: mentorPageSize, Status: "active"}}
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
	mustContain(t, "text", sent[0].Text, "Mia Mentee worked on Kernel <Dev>.\n\nMia Mentee learned")
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
			if len(sent) != 1 || sent[0].To != "mentee@linuxfoundation.org" || sent[0].Subject != "You have a new task for Kernel <Dev>" {
				t.Fatalf("sent = %+v", sent)
			}
			mustContain(t, "text", sent[0].Text, "Hi Mia!", "Task Name: Ship <it>\nTask Created On: October 5, 2026\n", "View your tasks: "+testSelfServeURL+"/mentorship")
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
