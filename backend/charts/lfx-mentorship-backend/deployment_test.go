// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package lfxmentorshipbackend_test

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// ArgoCD leaves hooks out of its OutOfSync calculation, so a roster-only change
// has to alter a normal resource or the approver-sync hook never runs. The
// checksum sits on the Deployment's own metadata, not the pod template, so it
// must not roll the pods.
func TestApproverTeamChecksumTracksRosterWithoutRollingPods(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}

	baseline := renderDeployment(t)
	cases := map[string][]string{
		"members changed": {"--set", "approverTeam.members={ada,grace,linus}"},
		"disabled":        {"--set", "approverTeam.enabled=false"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			changed := renderDeployment(t, args...)
			if approverChecksum(t, changed) == approverChecksum(t, baseline) {
				t.Error("checksum/approver-team did not change")
			}
			if podTemplate(t, changed) != podTemplate(t, baseline) {
				t.Error("pod template changed; a roster change must not roll the pods")
			}
		})
	}
}

// renderDeployment renders only the Deployment with the values every render
// needs plus a two-member roster, overridden by extra --set flags.
func renderDeployment(t *testing.T, extra ...string) string {
	t.Helper()
	args := []string{"template", "x", ".", "-s", "templates/deployment.yaml",
		"--set", "image.tag=t",
		"--set", "secretName=s",
		"--set", "config.HEIMDALL_JWKS_URL=https://h/jwks",
		"--set", "config.HEIMDALL_JWT_AUDIENCE=a",
		"--set", "config.HEIMDALL_JWT_ISSUER=i",
		"--set", "config.AWS_REGION=us-west-2",
		"--set", "approverTeam.enabled=true",
		"--set", "approverTeam.members={ada,grace}",
	}
	out, err := exec.Command("helm", append(args, extra...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	return string(out)
}

var approverChecksumLine = regexp.MustCompile(`checksum/approver-team: ([0-9a-f]{64})`)

// approverChecksum returns the annotation from the Deployment's own metadata,
// the part of the render before the pod template.
func approverChecksum(t *testing.T, rendered string) string {
	t.Helper()
	objectMeta, _ := splitAtPodTemplate(t, rendered)
	m := approverChecksumLine.FindStringSubmatch(objectMeta)
	if m == nil {
		t.Fatal("checksum/approver-team is missing from the Deployment metadata")
	}
	return m[1]
}

func podTemplate(t *testing.T, rendered string) string {
	t.Helper()
	_, template := splitAtPodTemplate(t, rendered)
	return template
}

func splitAtPodTemplate(t *testing.T, rendered string) (string, string) {
	t.Helper()
	at := strings.Index(rendered, "\n  template:\n")
	if at < 0 {
		t.Fatalf("rendered Deployment has no pod template:\n%s", rendered)
	}
	return rendered[:at], rendered[at:]
}
