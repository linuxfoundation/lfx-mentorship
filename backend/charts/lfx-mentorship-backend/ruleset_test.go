// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package lfxmentorshipbackend_test

import (
	"os"
	"strings"
	"testing"
)

func TestRuleSetDoesNotContainRetiredRoutesOrBroadProgramMethods(t *testing.T) {
	contents, err := os.ReadFile("templates/ruleset.yaml")
	if err != nil {
		t.Fatalf("read RuleSet: %v", err)
	}
	ruleset := string(contents)
	for _, retired := range []string{
		"/mentorship/v1/me/:profileType",
		"/mentorship/v1/program-terms/:id",
		"/mentorship/v1/internal/metrics",
		"/mentorship/v1/admin/approver-team/members",
	} {
		if strings.Contains(ruleset, retired) {
			t.Errorf("RuleSet contains retired route %q", retired)
		}
	}
	routeLines := map[string]bool{}
	for _, line := range strings.Split(ruleset, "\n") {
		routeLines[strings.TrimSpace(line)] = true
	}
	for _, profile := range []string{
		"- path: /mentorship/v1/mentors/:id",
		"- path: /mentorship/v1/mentees/:id",
	} {
		if !routeLines[profile] {
			t.Errorf("RuleSet is missing public directory profile route %q", profile)
		}
	}
	resolverStart := strings.Index(ruleset, "id: rule:lfx:lfx-mentorship-backend:public-resolver")
	if resolverStart < 0 {
		t.Fatal("RuleSet is missing the public resolver rule")
	}
	resolverEnd := strings.Index(ruleset[resolverStart:], "\n    - id:")
	if resolverEnd < 0 {
		t.Fatal("RuleSet public resolver rule has no following rule boundary")
	}
	resolverBlock := ruleset[resolverStart : resolverStart+resolverEnd]
	if strings.Count(resolverBlock, "- path:") != 1 || !strings.Contains(resolverBlock, "- path: /mentorship/v1/programs/resolve/:id") {
		t.Fatalf("public resolver rule contains unexpected routes:\n%s", resolverBlock)
	}
	if strings.Contains(ruleset, "methods: [PATCH, POST, DELETE]") {
		t.Fatal("RuleSet contains broad program mutation methods")
	}
	if !strings.Contains(ruleset, "/mentorship/v1/me/profiles/by-id/:id") {
		t.Fatal("RuleSet is missing profile-by-ID coverage")
	}
	for _, required := range []string{
		"object: mentorship_approver_team:global",
		"relation: reviewer",
		"relation: manager",
		"relation: assignee",
		"/mentorship/v1/programs/:programID/terms/:id/applications/bulk-decline",
	} {
		if !strings.Contains(ruleset, required) {
			t.Errorf("RuleSet is missing required authorization mapping %q", required)
		}
	}
}

func TestPublicCatalogRuleIsAnonymousReadOnly(t *testing.T) {
	block := ruleBlock(t, "programs-catalog-public")
	if strings.Count(block, "- path:") != 1 || !strings.Contains(block, "- path: /mentorship/v1/programs/catalog\n") {
		t.Fatalf("public catalog rule must cover only the catalog collection:\n%s", block)
	}
	assertAnonymousReadOnly(t, "public catalog", block)
}

func TestPublicCollectionsRuleIsAnonymousReadOnly(t *testing.T) {
	block := ruleBlock(t, "public-collections")
	paths := []string{
		"/mentorship/v1/programs",
		"/mentorship/v1/mentors",
		"/mentorship/v1/mentors/summary",
		"/mentorship/v1/mentees",
		"/mentorship/v1/mentees/summary",
		"/mentorship/v1/summary",
		"/mentorship/v1/funding-stats/total",
	}
	if got := strings.Count(block, "- path:"); got != len(paths) {
		t.Fatalf("public collections rule has %d routes, want %d:\n%s", got, len(paths), block)
	}
	for _, path := range paths {
		if !strings.Contains(block, "- path: "+path+"\n") {
			t.Errorf("public collections rule is missing %q", path)
		}
	}
	assertAnonymousReadOnly(t, "public collections", block)
}

// ruleBlock returns the RuleSet text of the rule with the given id suffix, up to the next rule.
func ruleBlock(t *testing.T, id string) string {
	t.Helper()
	contents, err := os.ReadFile("templates/ruleset.yaml")
	if err != nil {
		t.Fatalf("read RuleSet: %v", err)
	}
	ruleset := string(contents)
	start := strings.Index(ruleset, "id: rule:lfx:lfx-mentorship-backend:"+id+"\n")
	if start < 0 {
		t.Fatalf("RuleSet is missing rule %q", id)
	}
	end := strings.Index(ruleset[start:], "\n    - id:")
	if end < 0 {
		t.Fatalf("RuleSet rule %q has no following rule boundary", id)
	}
	return ruleset[start : start+end]
}

func assertAnonymousReadOnly(t *testing.T, name, block string) {
	t.Helper()
	if !strings.Contains(block, "methods: [GET]\n") {
		t.Errorf("%s rule must be GET-only", name)
	}
	for _, required := range []string{
		"- authenticator: anonymous_authenticator",
		"- authorizer: allow_all",
		"- finalizer: create_jwt",
	} {
		if !strings.Contains(block, required) {
			t.Errorf("%s rule is missing %q", name, required)
		}
	}
}

func TestReviewerFieldsRequireReviewerRelation(t *testing.T) {
	contents, err := os.ReadFile("templates/ruleset.yaml")
	if err != nil {
		t.Fatalf("read RuleSet: %v", err)
	}
	ruleset := string(contents)
	start := strings.Index(ruleset, "id: rule:lfx:lfx-mentorship-backend:application-reviewer-fields")
	if start < 0 {
		t.Fatal("RuleSet is missing application reviewer-fields rule")
	}
	block := ruleset[start:]
	for _, route := range []string{
		"/mentorship/v1/applications/:id/note",
		"/mentorship/v1/applications/:id/evaluation",
	} {
		if !strings.Contains(block, route) {
			t.Errorf("reviewer-fields rule is missing %q", route)
		}
	}
	if !strings.Contains(block, "relation: reviewer") {
		t.Fatal("reviewer-fields rule does not require reviewer relation")
	}
}

func TestManagementReadsRequireManagerAuthorization(t *testing.T) {
	contents, err := os.ReadFile("templates/ruleset.yaml")
	if err != nil {
		t.Fatalf("read RuleSet: %v", err)
	}
	ruleset := string(contents)
	for _, route := range []string{
		"/mentorship/v1/programs/:id/management-summary",
		"/mentorship/v1/programs/:id/member-management",
		"/mentorship/v1/programs/:id/term-management",
	} {
		start := strings.Index(ruleset, route)
		if start < 0 {
			t.Fatalf("RuleSet is missing management route %q", route)
		}
		end := strings.Index(ruleset[start:], "\n    - id:")
		if end < 0 {
			end = len(ruleset) - start
		}
		block := ruleset[start : start+end]
		expected := `      execute:
        - authenticator: oidc
        - authorizer: openfga_check
          config:
            values:
              object: 'mentorship_program:{{ "{{- .Request.URL.Captures.id -}}" }}'
              relation: manager
        - finalizer: create_jwt`
		if !strings.Contains(block, expected) {
			t.Errorf("management route %q lacks oidc -> openfga manager -> create_jwt sequence", route)
		}
	}
}

func TestMentorModuleRoutesAreCoveredByHeimdall(t *testing.T) {
	contents, err := os.ReadFile("templates/ruleset.yaml")
	if err != nil {
		t.Fatalf("read RuleSet: %v", err)
	}
	ruleset := string(contents)
	for _, route := range []string{
		"/mentorship/v1/me/profiles",
		"/mentorship/v1/me/profiles/:profileType",
		"/mentorship/v1/me/applications",
		"/mentorship/v1/mentor-invites/:token/accept",
		"/mentorship/v1/mentor-invites/:token/decline",
		"/mentorship/v1/programs/:programID/terms/:id/applications",
		"/mentorship/v1/applications/:id/withdraw",
		"/mentorship/v1/applications/:id/note",
		"/mentorship/v1/applications/:id/tasks",
		"/mentorship/v1/tasks/:id/review",
	} {
		if !strings.Contains(ruleset, route) {
			t.Errorf("RuleSet is missing mentor module route %q", route)
		}
	}

	for _, required := range []string{
		"id: rule:lfx:lfx-mentorship-backend:mentor-invites",
		"id: rule:lfx:lfx-mentorship-backend:term-application-create",
		"id: rule:lfx:lfx-mentorship-backend:applications-mentee",
		"id: rule:lfx:lfx-mentorship-backend:applications-reviewer",
		"id: rule:lfx:lfx-mentorship-backend:application-reviewer-fields",
		"id: rule:lfx:lfx-mentorship-backend:tasks-manager",
	} {
		if !strings.Contains(ruleset, required) {
			t.Errorf("RuleSet is missing mentor module authorization group %q", required)
		}
	}
}
