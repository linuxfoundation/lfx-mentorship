// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package lfxmentorshipbackend_test

import (
	"os"
	"strings"
	"testing"
)

// ArgoCD leaves hooks out of its OutOfSync calculation, so a roster-only change
// has to touch a normal resource or the approver-sync hook never runs. The
// checksum sits on the Deployment's own metadata, not the pod template, so it
// does not roll the pods.
func TestApproverTeamChecksumIsOnDeploymentMetadataOnly(t *testing.T) {
	contents, err := os.ReadFile("templates/deployment.yaml")
	if err != nil {
		t.Fatalf("read Deployment: %v", err)
	}
	deployment := string(contents)

	specAt := strings.Index(deployment, "\nspec:")
	if specAt < 0 {
		t.Fatal("Deployment has no spec")
	}
	objectMeta, podTemplate := deployment[:specAt], deployment[specAt:]

	if !strings.Contains(objectMeta, "checksum/approver-team:") {
		t.Error("Deployment metadata lacks the checksum/approver-team annotation")
	}
	if strings.Contains(podTemplate, "checksum/approver-team") {
		t.Error("checksum/approver-team must not be in the pod template; it would roll the pods on every roster change")
	}
	for _, input := range []string{".Values.approverTeam.enabled", ".Values.approverTeam.members"} {
		if !strings.Contains(objectMeta, input) {
			t.Errorf("checksum/approver-team does not cover %s", input)
		}
	}
}
