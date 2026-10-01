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
	templateMentorInvited       = "mentor_invited"
	templateMentorDeclined      = "mentor_declined"
	templateAdminMentorDeclined = "admin_mentor_declined"
	templateAdminMentorAccepted = "admin_mentor_accepted"
	templateAdminTasksSubmitted = "admin_tasks_submitted"
	templateMenteeAccepted      = "mentee_accepted"
	templateAdminMenteeAccepted = "admin_mentee_accepted"
)

// templateData is the union of fields the notification templates read.
type templateData struct {
	RecipientName  string
	ProgramName    string
	ProgramURL     string
	ManageURL      string
	HRNotice       bool
	TermName       string
	TermPeriod     string
	MenteeName     string
	MenteeEmail    string
	MenteeCountry  string
	MentorName     string
	AttendanceType string
	ProgramAdmins  []string
	InviteURL      string
	LFID           string
	Year           int
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
