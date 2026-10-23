// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/nats-io/nats.go"
)

// fakeProject is a Project Service project.
type fakeProject struct {
	Slug, Name, Logo string
}

// fakeAccount is an LF account known to auth-service.
type fakeAccount struct {
	Username, Email, Name string
}

// fakes answers the NATS and HTTP calls the API makes to other LFX services.
type fakes struct {
	mu           sync.Mutex
	emails       []emailapi.SendEmailRequest
	projects     map[string]fakeProject
	accounts     []fakeAccount
	transactions map[string][]map[string]any
	// crowdfundingDown makes the crowdfunding fake answer 502.
	crowdfundingDown bool
	// granted is fga-sync's view of who has a tuple on which object, keyed
	// "{object_type}:{uid}" → username → when the relay's message arrived.
	granted map[string]map[string]time.Time
	// accessHold delays granting access after a tuple arrives; the test clock for issue #354.
	accessHold time.Duration
	// accessChecks records every lfx.access_check.request body and the answer given.
	accessChecks []accessCheck
}

// accessCheck is one access check the API asked fga-sync, and the answer it got.
type accessCheck struct {
	Check   string
	Allowed bool
}

func startFakes(nc *nats.Conn) (*fakes, error) {
	f := &fakes{}
	f.reset()
	subs := map[string]nats.MsgHandler{
		emailapi.SendEmailSubject:             f.sendEmail,
		constants.ProjectGetSlugSubject:       f.project(func(p fakeProject) string { return p.Slug }),
		constants.ProjectGetNameSubject:       f.project(func(p fakeProject) string { return p.Name }),
		constants.ProjectGetLogoSubject:       f.project(func(p fakeProject) string { return p.Logo }),
		"lfx.auth-service.email_to_username":  f.emailToUsername,
		"lfx.auth-service.user_metadata.read": f.userMetadata,
		"lfx.auth-service.user_emails.read":   f.userEmails,
		"lfx.fga-sync.>":                      f.recordTuples,
		"lfx.access_check.request":            f.accessCheck,
	}
	for subject, handler := range subs {
		if _, err := nc.Subscribe(subject, handler); err != nil {
			return nil, fmt.Errorf("subscribe %s: %w", subject, err)
		}
	}
	return f, nc.Flush()
}

func (f *fakes) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emails = nil
	f.projects = map[string]fakeProject{}
	f.accounts = nil
	f.transactions = map[string][]map[string]any{}
	f.crowdfundingDown = false
	f.granted = map[string]map[string]time.Time{}
	f.accessHold = 0
	f.accessChecks = nil
}

func (f *fakes) addProject(uid string, p fakeProject) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.projects[uid] = p
}

func (f *fakes) addAccount(a fakeAccount) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts = append(f.accounts, a)
}

func (f *fakes) setTransactions(programID string, txns []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transactions[programID] = txns
}

func (f *fakes) setCrowdfundingDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.crowdfundingDown = down
}

// holdAccess makes the fake fga-sync grant a user's access only hold after the
// relay's tuple arrives, standing in for the sync lag behind issue #354.
func (f *fakes) holdAccess(hold time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accessHold = hold
}

// accessChecksFor returns the access checks asked about object, in order.
func (f *fakes) accessChecksFor(object string) []accessCheck {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []accessCheck
	for _, c := range f.accessChecks {
		if strings.HasPrefix(c.Check, object+"#") {
			out = append(out, c)
		}
	}
	return out
}

// recordTuples plays fga-sync's consumer: every sync message that names a user
// on an object records that the user now has a tuple there. Relations are not
// modelled; an access check passes once the user has any tuple on the object.
func (f *fakes) recordTuples(msg *nats.Msg) {
	var m struct {
		ObjectType string `json:"object_type"`
		Data       struct {
			UID       string              `json:"uid"`
			Username  string              `json:"username"`
			Relations map[string][]string `json:"relations"`
		} `json:"data"`
	}
	if json.Unmarshal(msg.Data, &m) != nil || m.Data.UID == "" {
		return
	}
	users := []string{}
	if m.Data.Username != "" {
		users = append(users, m.Data.Username)
	}
	for _, names := range m.Data.Relations {
		users = append(users, names...)
	}
	object := m.ObjectType + ":" + m.Data.UID
	now := time.Now()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, user := range users {
		if f.granted[object] == nil {
			f.granted[object] = map[string]time.Time{}
		}
		if _, seen := f.granted[object][user]; !seen {
			f.granted[object][user] = now
		}
	}
}

// accessCheck answers lfx.access_check.request as fga-sync does: one
// "{check}\t{true|false}" line per "object#relation@user:name" line asked.
func (f *fakes) accessCheck(msg *nats.Msg) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var lines []string
	for _, check := range strings.Split(strings.TrimSpace(string(msg.Data)), "\n") {
		object, rest, _ := strings.Cut(check, "#")
		_, user, _ := strings.Cut(rest, "@user:")
		arrived, ok := f.granted[object][user]
		allowed := ok && time.Since(arrived) >= f.accessHold
		f.accessChecks = append(f.accessChecks, accessCheck{Check: check, Allowed: allowed})
		lines = append(lines, fmt.Sprintf("%s\t%t", check, allowed))
	}
	_ = msg.Respond([]byte(strings.Join(lines, "\n")))
}

// sentEmails returns the emails sent so far.
func (f *fakes) sentEmails() []emailapi.SendEmailRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]emailapi.SendEmailRequest(nil), f.emails...)
}

// emailsTo returns the emails sent to address.
func (f *fakes) emailsTo(address string) []emailapi.SendEmailRequest {
	var out []emailapi.SendEmailRequest
	for _, e := range f.sentEmails() {
		if strings.EqualFold(e.To, address) {
			out = append(out, e)
		}
	}
	return out
}

func (f *fakes) sendEmail(msg *nats.Msg) {
	var req emailapi.SendEmailRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		reply(msg, emailapi.SendEmailErrorResponse{Error: err.Error()})
		return
	}
	f.mu.Lock()
	f.emails = append(f.emails, req)
	n := len(f.emails)
	f.mu.Unlock()
	reply(msg, emailapi.SendEmailResponse{EmailID: fmt.Sprintf("e-%d", n), GroupID: "g"})
}

func (f *fakes) project(field func(fakeProject) string) nats.MsgHandler {
	return func(msg *nats.Msg) {
		f.mu.Lock()
		p, ok := f.projects[string(msg.Data)]
		f.mu.Unlock()
		if !ok {
			reply(msg, events.RPCError{Code: events.RPCErrorNotFound, Message: "project not found"})
			return
		}
		_ = msg.Respond([]byte(field(p)))
	}
}

func (f *fakes) account(match func(fakeAccount) bool) (fakeAccount, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.accounts {
		if match(a) {
			return a, true
		}
	}
	return fakeAccount{}, false
}

func (f *fakes) emailToUsername(msg *nats.Msg) {
	email := strings.TrimSpace(string(msg.Data))
	a, ok := f.account(func(a fakeAccount) bool { return strings.EqualFold(a.Email, email) })
	if !ok {
		reply(msg, map[string]any{"success": false, "error": "user not found"})
		return
	}
	_ = msg.Respond([]byte(a.Username))
}

func (f *fakes) userMetadata(msg *nats.Msg) {
	username := strings.TrimSpace(string(msg.Data))
	a, ok := f.account(func(a fakeAccount) bool { return a.Username == username })
	if !ok {
		reply(msg, map[string]any{"success": false, "error": "user not found"})
		return
	}
	reply(msg, map[string]any{"success": true, "data": map[string]string{"name": a.Name}})
}

func (f *fakes) userEmails(msg *nats.Msg) {
	var req struct {
		User struct {
			AuthToken string `json:"auth_token"`
		} `json:"user"`
	}
	_ = json.Unmarshal(msg.Data, &req)
	a, ok := f.account(func(a fakeAccount) bool { return a.Username == req.User.AuthToken })
	if !ok {
		reply(msg, map[string]any{"success": false, "error": "user not found"})
		return
	}
	reply(msg, map[string]any{"success": true, "data": map[string]any{"primary_email": a.Email, "alternate_emails": []string{}}})
}

func reply(msg *nats.Msg, body any) {
	data, _ := json.Marshal(body)
	_ = msg.Respond(data)
}

// crowdfundingHandler serves GET /initiatives/{id}/transactions from the configured transactions.
func (f *fakes) crowdfundingHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /initiatives/{id}/transactions", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		down := f.crowdfundingDown
		txns := f.transactions[r.PathValue("id")]
		f.mu.Unlock()
		if down {
			http.Error(w, "upstream down", http.StatusBadGateway)
			return
		}
		if txns == nil {
			txns = []map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": txns, "total_count": len(txns), "limit": 100, "offset": 0})
	})
	return mux
}
