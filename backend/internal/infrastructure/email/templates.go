// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
	"time"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// templateFuncs are shared by the HTML and text templates.
var templateFuncs = map[string]any{"detail": newDetail, "action": newAction}

var (
	htmlTemplates = htmltemplate.Must(htmltemplate.New("").Funcs(templateFuncs).ParseFS(templateFS, "templates/*.html.tmpl"))
	textTemplates = texttemplate.Must(texttemplate.New("").Funcs(templateFuncs).ParseFS(templateFS, "templates/*.txt.tmpl"))
)

// detail is one labelled row of a template's details block.
type detail struct {
	Label string
	Value string
}

func newDetail(label, value string) detail {
	return detail{Label: label, Value: value}
}

// action is one button in a template's row of buttons. Variant picks its style in the layout:
// primary, secondary, approve, or reject.
type action struct {
	URL     string
	Label   string
	Variant string
}

func newAction(url, label, variant string) action {
	return action{URL: url, Label: label, Variant: variant}
}

// taskItem is a task listed in an email, with its due date when one is known.
type taskItem struct {
	Name    string
	DueDate string
}

// adminContact is a program admin listed in the HR acceptance notice.
type adminContact struct {
	Name  string
	Email string
}

const (
	templateProgramSubmitted           = "program_submitted"
	templateProgramReview              = "program_review"
	templateProgramApproved            = "program_approved"
	templateProgramRejected            = "program_rejected"
	templateProgramUpdated             = "program_updated"
	templateMentorProfileCreated       = "mentor_profile_created"
	templateMenteeProfileCreated       = "mentee_profile_created"
	templateMentorInvited              = "mentor_invited"
	templateMentorDeclined             = "mentor_declined"
	templateAdminMentorDeclined        = "admin_mentor_declined"
	templateAdminMentorAccepted        = "admin_mentor_accepted"
	templateAdminMentorRequested       = "admin_mentor_requested"
	templateAdminMentorWithdrew        = "admin_mentor_withdrew"
	templateAdminTasksSubmitted        = "admin_tasks_submitted"
	templateMenteeApplicationReceived  = "mentee_application_received"
	templateMenteeAccepted             = "mentee_accepted"
	templateAdminMenteeAccepted        = "admin_mentee_accepted"
	templateMenteeDeclined             = "mentee_declined"
	templateMenteeWithdrawn            = "mentee_withdrawn"
	templateAdminMenteeWithdrawn       = "admin_mentee_withdrawn"
	templateMenteeGraduated            = "mentee_graduated"
	templateMenteeTaskAssigned         = "mentee_task_assigned"
	templateMenteePrerequisiteReminder = "mentee_prerequisite_reminder"
)

// templateData is the union of fields the notification templates read.
type templateData struct {
	// Letter drops the sign-off, for a template that is a formal letter.
	Letter             bool
	RecipientName      string
	ProgramName        string
	ProgramDescription string
	ProgramURL         string
	ManageURL          string
	SupportURL         string
	ApproveURL         string
	RejectURL          string
	ProfileURL         string
	TermName           string
	TermStart          string
	TermEnd            string
	ApplicationEndDate string
	MenteeName         string
	MenteeEmail        string
	MenteeCountry      string
	MentorName         string
	MentorEmail        string
	MentorNames        []string
	InviterName        string
	SubmitterName      string
	SubmitterEmail     string
	AttendanceType     string
	ProgramAdmins      []adminContact
	Tasks              []taskItem
	TaskName           string
	TaskCreatedOn      string
	TaskDueDate        string
	IssuedDate         string
	InviteURL          string
	LFID               string
	Year               int
}

// render builds a Message from the named template pair.
func render(name, to, subject string, data templateData) (Message, error) {
	data.Year = time.Now().UTC().Year()
	var html, text bytes.Buffer
	if err := htmlTemplates.ExecuteTemplate(&html, name+".html.tmpl", data); err != nil {
		return Message{}, fmt.Errorf("render %s html: %w", name, err)
	}
	if err := textTemplates.ExecuteTemplate(&text, name+".txt.tmpl", data); err != nil {
		return Message{}, fmt.Errorf("render %s text: %w", name, err)
	}
	return Message{
		To:      to,
		Subject: subject,
		HTML:    html.String(),
		Text:    text.String(),
	}, nil
}
