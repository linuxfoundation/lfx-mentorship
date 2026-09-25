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
		"- path: /mentorship/v1/programs/catalog",
		"- path: /mentorship/v1/mentors",
		"- path: /mentorship/v1/mentees",
		"- path: /mentorship/v1/summary",
		"- path: /mentorship/v1/funding-stats/total",
	} {
		if strings.Contains(ruleset, retired) {
			t.Errorf("RuleSet contains retired route %q", retired)
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
