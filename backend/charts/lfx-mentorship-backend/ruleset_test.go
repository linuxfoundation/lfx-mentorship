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
	} {
		if strings.Contains(ruleset, retired) {
			t.Errorf("RuleSet contains retired route %q", retired)
		}
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
