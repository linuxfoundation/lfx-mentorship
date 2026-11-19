// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package lfxmentorshipbackend_test

import (
	"os/exec"
	"regexp"
	"testing"
)

// renderProjectSync renders only the project-sync CronJob, enabled, with the values
// every render needs, overridden by extra --set flags.
func renderProjectSync(t *testing.T, extra ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	args := []string{"template", "x", ".", "-s", "templates/cronjob-project-sync.yaml",
		"--set", "image.tag=t",
		"--set", "secretName=s",
		"--set", "config.HEIMDALL_JWKS_URL=https://h/jwks",
		"--set", "config.HEIMDALL_JWT_AUDIENCE=a",
		"--set", "config.HEIMDALL_JWT_ISSUER=i",
		"--set", "config.AWS_REGION=us-west-2",
		"--set", "config.FGA_NATS_URL=nats://n:4222",
		"--set", "config.PUBLIC_SITE_URL=https://p.example",
		"--set", "config.SELF_SERVE_URL=https://s.example",
		"--set", "config.EMAIL_COPY_INBOX=c@example.org",
		"--set", "config.EMAIL_REVIEW_INBOX=r@example.org",
		"--set", "config.EMAIL_SEND_RATE=4",
		"--set", "projectSyncCronJob.enabled=true",
	}
	out, err := exec.Command("helm", append(args, extra...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	return string(out)
}

var dryRunEnv = regexp.MustCompile(`- name: DRY_RUN\s+value: "([^"]*)"`)

// DRY_RUN in the shared env: list also reaches program-funding-stats-sync, so the
// project sync takes its dry run from projectSyncCronJob.dryRun alone (#465 review).
func TestProjectSyncDryRunIsScopedToTheJob(t *testing.T) {
	sharedDryRun := []string{"--set", "env[0].name=DRY_RUN", "--set-string", "env[0].value=true", "--set", "env[1].name=OTHER", "--set-string", "env[1].value=kept"}
	for name, tc := range map[string]struct {
		flags []string
		want  string
	}{
		"default":                         {want: "false"},
		"dryRun set":                      {flags: []string{"--set", "projectSyncCronJob.dryRun=true"}, want: "true"},
		"shared DRY_RUN is not inherited": {flags: sharedDryRun, want: "false"},
	} {
		t.Run(name, func(t *testing.T) {
			rendered := renderProjectSync(t, tc.flags...)
			matches := dryRunEnv.FindAllStringSubmatch(rendered, -1)
			if len(matches) != 1 || matches[0][1] != tc.want {
				t.Fatalf("DRY_RUN entries = %v; want exactly one, %q", matches, tc.want)
			}
		})
	}

	if rendered := renderProjectSync(t, sharedDryRun...); !regexp.MustCompile(`- name: OTHER\s+value: kept`).MatchString(rendered) {
		t.Fatal("the other shared env: entries were dropped")
	}
}
