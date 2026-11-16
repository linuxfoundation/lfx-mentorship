// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package lfxmentorshipbackend_test

import (
	"os/exec"
	"strings"
	"testing"
)

// Every pod the chart runs carries the Datadog service tag, so the API's and the jobs' logs are
// all filed under one service and the platform's per-service error monitors count them.
func TestPodTemplatesCarryDatadogServiceTag(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	const want = "tags.datadoghq.com/service: lfx-mentorship-backend"
	for template, enable := range map[string][]string{
		"templates/deployment.yaml":                    nil,
		"templates/job-migrate.yaml":                   nil,
		"templates/job-approver-sync.yaml":             {"--set", "approverTeam.enabled=true", "--set", "approverTeam.members={ada}"},
		"templates/cronjob-term-status.yaml":           {"--set", "termStatusCronJob.enabled=true"},
		"templates/cronjob-prerequisite-reminder.yaml": {"--set", "prerequisiteReminderCronJob.enabled=true"},
		"templates/cronjob-funding-stats-sync.yaml":    {"--set", "fundingStatsSyncCronJob.enabled=true", "--set", "config.LEDGER_BASE_URL=https://ledger.example.org"},
	} {
		t.Run(template, func(t *testing.T) {
			_, podTemplate, ok := strings.Cut(render(t, template, enable...), "template:")
			if !ok || !strings.Contains(podTemplate, want) {
				t.Fatalf("pod template lacks %q", want)
			}
		})
	}
}
