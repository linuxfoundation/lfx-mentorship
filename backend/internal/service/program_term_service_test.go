// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

func TestProgramTermService_ListByProgram_RejectsNonPublicStatus(t *testing.T) {
	svc := service.NewProgramTermService(&stubTermRepo{}, &stubAppRepo{})
	for _, status := range []string{"deleted", "bogus"} {
		_, _, err := svc.ListByProgram(context.Background(), "p1", models.ProgramTermFilter{Status: status})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("status %q: err=%v; want ErrInvalidInput", status, err)
		}
	}
}
