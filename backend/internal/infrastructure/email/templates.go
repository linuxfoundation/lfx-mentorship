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

var (
	htmlTemplates = htmltemplate.Must(htmltemplate.ParseFS(templateFS, "templates/*.html.tmpl"))
	textTemplates = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/*.txt.tmpl"))
)

const (
	templateProgramSubmitted          = "program_submitted"
	templateProgramReview             = "program_review"
	templateProgramApproved           = "program_approved"
	templateProgramRejected           = "program_rejected"
	templateMentorInvited             = "mentor_invited"
	templateMentorDeclined            = "mentor_declined"
	templateAdminMentorDeclined       = "admin_mentor_declined"
	templateAdminMentorAccepted       = "admin_mentor_accepted"
	templateAdminMentorRequested      = "admin_mentor_requested"
	templateAdminMentorWithdrew       = "admin_mentor_withdrew"
	templateAdminTasksSubmitted       = "admin_tasks_submitted"
	templateMenteeApplicationReceived = "mentee_application_received"
	templateMenteeAccepted            = "mentee_accepted"
	templateAdminMenteeAccepted       = "admin_mentee_accepted"
	templateMenteeDeclined            = "mentee_declined"
	templateMenteeWithdrawn           = "mentee_withdrawn"
	templateAdminMenteeWithdrawn      = "admin_mentee_withdrawn"
	templateMenteeGraduated           = "mentee_graduated"
	templateMenteeTaskAssigned        = "mentee_task_assigned"
)

// templateData is the union of fields the notification templates read.
type templateData struct {
	// Letter drops the greeting and sign-off, for a template that is a formal letter.
	Letter             bool
	RecipientName      string
	ProgramName        string
	ProgramDescription string
	ProgramURL         string
	ManageURL          string
	SupportURL         string
	HRNotice           bool
	TermName           string
	TermPeriod         string
	MenteeName         string
	MenteeEmail        string
	MenteeCountry      string
	MentorName         string
	MentorEmail        string
	MentorNames        []string
	SubmitterName      string
	SubmitterEmail     string
	AttendanceType     string
	ProgramAdmins      []string
	Tasks              []string
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
