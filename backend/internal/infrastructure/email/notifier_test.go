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
)

type senderStub struct {
	mu      sync.Mutex
	sent    []Message
	failFor map[string]bool
	release chan struct{}
}

func (s *senderStub) Send(_ context.Context, msg Message) (Receipt, error) {
	if s.release != nil {
		<-s.release
	}
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
	members []*models.ProgramMember
	filters []models.ProgramMemberFilter
}

func (r *memberRepoStub) ListByProgram(_ context.Context, _ string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	r.filters = append(r.filters, filter)
	start := min(filter.Offset, len(r.members))
	end := min(start+filter.Limit, len(r.members))
	return r.members[start:end], &models.PaginationMeta{Total: len(r.members), Limit: filter.Limit, Offset: filter.Offset}, nil
}

type applicationRepoStub struct {
	domain.ApplicationRepository
	app *models.Application
}

func (r *applicationRepoStub) GetByID(context.Context, string) (*models.Application, error) {
	return r.app, nil
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
	}
	f.n = NewNotifier(f.sender, Repositories{
		Users:    f.users,
		Profiles: f.profiles,
		Programs: &programRepoStub{program: &models.Program{ID: "p1", Name: "Kernel <Dev>", Slug: "kernel-dev"}},
		Terms: &termRepoStub{term: &models.ProgramTerm{
			ID: "t1", ProgramID: "p1", Name: "Fall 2026",
			StartDateTime: timePtr(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)),
			EndDateTime:   timePtr(time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC)),
		}},
		Members:      f.members,
		Applications: &applicationRepoStub{app: &models.Application{ID: "a1", ProgramTermID: "t1", UserID: "mentee"}},
	}, NotifierConfig{PublicSiteURL: testPublicSiteURL + "/", SelfServeURL: testSelfServeURL, HRInbox: testHRInbox}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	want := []models.ProgramMemberFilter{{Limit: adminPageSize, MemberType: "program_admin", Status: "active"}}
	if !slices.Equal(f.members.filters, want) {
		t.Fatalf("filters = %+v, want %+v", f.members.filters, want)
	}
}

func TestNotifyAdminTasksSubmitted_PagesThroughAllAdmins(t *testing.T) {
	f := newFixture()
	for i := range adminPageSize + 1 {
		id := "page-admin-" + strconv.Itoa(i)
		f.users.users[id] = &models.User{ID: id, Email: strPtr(id + "@linuxfoundation.org")}
		f.members.members = append(f.members.members, &models.ProgramMember{UserID: id})
	}

	f.n.NotifyAdminTasksSubmitted(context.Background(), "a1")

	if sent := f.wait(t); len(sent) != adminPageSize+1 {
		t.Fatalf("sent %d messages, want %d", len(sent), adminPageSize+1)
	}
	if len(f.members.filters) != 2 || f.members.filters[1].Offset != adminPageSize {
		t.Fatalf("filters = %+v, want a second page at offset %d", f.members.filters, adminPageSize)
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
