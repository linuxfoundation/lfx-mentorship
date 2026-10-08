// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

const cdnPrefix = "https://cdn.example.org/mentorship"

var (
	pngBytes = []byte("\x89PNG\r\n\x1a\nimage")
	pdfBytes = []byte("%PDF-1.7\n\x00body")
)

type putCall struct {
	key, contentType, cacheControl string
	size                           int
}

type fakeObjectStore struct {
	puts    []putCall
	putErr  error
	getKeys []string
	object  *domain.StoredObject
	getErr  error
}

func (f *fakeObjectStore) PutObject(_ context.Context, key string, data []byte, contentType, cacheControl string) error {
	f.puts = append(f.puts, putCall{key, contentType, cacheControl, len(data)})
	return f.putErr
}

func (f *fakeObjectStore) GetObject(_ context.Context, key, _ string) (*domain.StoredObject, error) {
	f.getKeys = append(f.getKeys, key)
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.object, nil
}

type scheduled struct {
	bucket  domain.ObjectBucket
	locator string
	delay   time.Duration
}

type fakeScheduler struct {
	entries []scheduled
}

func (f *fakeScheduler) Schedule(_ context.Context, bucket domain.ObjectBucket, locator string, delay time.Duration) (string, error) {
	f.entries = append(f.entries, scheduled{bucket, locator, delay})
	return "pending-1", nil
}

type clearCall struct{ id, previous string }

type fakeFileRepo struct {
	programLogo []domain.FileReplacement
	taskFile    []domain.FileReplacement
	cleared     []clearCall
	replaceErr  error
}

func (f *fakeFileRepo) ReplaceProgramLogo(_ context.Context, r domain.FileReplacement) error {
	f.programLogo = append(f.programLogo, r)
	return f.replaceErr
}

func (f *fakeFileRepo) ReplaceTaskFile(_ context.Context, r domain.FileReplacement) error {
	f.taskFile = append(f.taskFile, r)
	return f.replaceErr
}

func (f *fakeFileRepo) ClearProgramLogo(_ context.Context, id, previous string) error {
	f.cleared = append(f.cleared, clearCall{id, previous})
	return nil
}

func (f *fakeFileRepo) ClearTaskFile(_ context.Context, id, previous string) error {
	f.cleared = append(f.cleared, clearCall{id, previous})
	return nil
}

type stubTaskAccess struct {
	task *models.Task
	err  error
}

func (s *stubTaskAccess) GetByIDForActor(context.Context, string, string) (*models.Task, error) {
	return s.task, s.err
}

func storedObject(contentType, cacheControl string) *domain.StoredObject {
	return &domain.StoredObject{
		Body:          io.NopCloser(bytes.NewReader([]byte("x"))),
		ContentType:   contentType,
		CacheControl:  cacheControl,
		ContentLength: 1,
	}
}

func ptr(s string) *string { return &s }

// fileSvc builds a FileService; deps left nil fall back to empty fakes.
type fileSvcDeps struct {
	files       *fakeFileRepo
	scheduler   *fakeScheduler
	programs    *stubProgRepo
	tasks       *stubTaskAccess
	logos       *fakeObjectStore
	attachments *fakeObjectStore
}

func (d *fileSvcDeps) build() *service.FileService {
	if d.files == nil {
		d.files = &fakeFileRepo{}
	}
	if d.scheduler == nil {
		d.scheduler = &fakeScheduler{}
	}
	if d.programs == nil {
		d.programs = &stubProgRepo{}
	}
	if d.tasks == nil {
		d.tasks = &stubTaskAccess{}
	}
	var logos *service.LogoBucket
	if d.logos != nil {
		logos = &service.LogoBucket{Store: d.logos, CDNURLPrefix: cdnPrefix}
	}
	var attachments domain.ObjectStore
	if d.attachments != nil {
		attachments = d.attachments
	}
	return service.NewFileService(d.files, d.scheduler, d.programs, d.tasks, logos, attachments)
}

func TestFileService_UploadProgramLogo_ReplaceProtocol(t *testing.T) {
	previous := cdnPrefix + "/old-logo.png"
	d := &fileSvcDeps{
		logos: &fakeObjectStore{},
		programs: &stubProgRepo{getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, LogoURL: &previous}, nil
		}},
	}
	got, err := d.build().UploadProgramLogo(context.Background(), "p1", pngBytes)
	if err != nil {
		t.Fatalf("UploadProgramLogo: %v", err)
	}

	put := d.logos.puts[0]
	if put.contentType != "image/png" || put.cacheControl != "public, max-age=86400" || !strings.HasSuffix(put.key, "-logo.png") {
		t.Fatalf("unexpected put %+v", put)
	}
	if got.PublicURL != cdnPrefix+"/"+put.key || got.Filename != "logo.png" || got.Size != int64(len(pngBytes)) {
		t.Fatalf("unexpected response %+v", got)
	}
	// Step 1: the new locator is queued for deletion after a grace period, before the write.
	if len(d.scheduler.entries) != 1 || d.scheduler.entries[0].locator != got.PublicURL ||
		d.scheduler.entries[0].bucket != domain.ObjectBucketLogos || d.scheduler.entries[0].delay <= 2*time.Minute {
		t.Fatalf("grace-period entry = %+v", d.scheduler.entries)
	}
	// Step 3: the row swap carries the previous value and the entry to cancel.
	rep := d.files.programLogo[0]
	if rep.RowID != "p1" || *rep.Previous != previous || rep.Next != got.PublicURL || rep.PendingDeletionID != "pending-1" {
		t.Fatalf("unexpected replacement %+v", rep)
	}
}

func TestFileService_UploadProgramLogo_FailedWriteLeavesRowAlone(t *testing.T) {
	d := &fileSvcDeps{logos: &fakeObjectStore{putErr: domain.ErrUpstreamUnavailable}}
	if _, err := d.build().UploadProgramLogo(context.Background(), "p1", pngBytes); !errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Fatalf("err = %v; want ErrUpstreamUnavailable", err)
	}
	if len(d.files.programLogo) != 0 {
		t.Fatal("row must not change when the object write fails")
	}
	if len(d.scheduler.entries) != 1 {
		t.Fatal("the grace-period entry must exist to reclaim a partial write")
	}
}

func TestFileService_UploadProgramLogo_LostRaceIsConflict(t *testing.T) {
	d := &fileSvcDeps{logos: &fakeObjectStore{}, files: &fakeFileRepo{replaceErr: domain.ErrConflict}}
	if _, err := d.build().UploadProgramLogo(context.Background(), "p1", pngBytes); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v; want ErrConflict", err)
	}
	if len(d.scheduler.entries) != 1 {
		t.Fatal("the losing upload's grace entry must remain to reclaim its object")
	}
}

func TestFileService_ArchivedProgramLogoIsLocked(t *testing.T) {
	logo := "https://cdn.example.org/mentorship/old.png"
	d := &fileSvcDeps{logos: &fakeObjectStore{}, programs: &stubProgRepo{getByID: func(_ context.Context, id string) (*models.Program, error) {
		return &models.Program{ID: id, Status: models.ProgramStatusArchived, LogoURL: &logo}, nil
	}}}
	svc := d.build()
	if _, err := svc.UploadProgramLogo(context.Background(), "p1", pngBytes); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("upload err = %v; want ErrStateLocked", err)
	}
	if err := svc.DeleteProgramLogo(context.Background(), "p1"); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("delete err = %v; want ErrStateLocked", err)
	}
	if len(d.scheduler.entries) != 0 {
		t.Fatal("a locked upload must not store or schedule anything")
	}
}

func TestFileService_UploadProgramLogo_RejectsBeforeStoring(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), domain.ErrUnsupportedMedia},
		{"too large", append(append([]byte{}, pngBytes...), make([]byte, service.MaxLogoBytes)...), domain.ErrPayloadTooLarge},
		{"empty", nil, domain.ErrInvalidInput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &fileSvcDeps{logos: &fakeObjectStore{}}
			_, err := d.build().UploadProgramLogo(context.Background(), "p1", tc.data)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if len(d.logos.puts) != 0 || len(d.scheduler.entries) != 0 || len(d.files.programLogo) != 0 {
				t.Fatal("rejected upload must not be queued or stored")
			}
		})
	}
}

func TestFileService_NotConfigured(t *testing.T) {
	svc := (&fileSvcDeps{}).build()
	if _, err := svc.UploadProgramLogo(context.Background(), "p1", pngBytes); !errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Fatalf("logo err = %v; want ErrUpstreamUnavailable", err)
	}
	if _, err := svc.UploadTaskFile(context.Background(), "t1", "u1", "a.pdf", pdfBytes); !errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Fatalf("task err = %v; want ErrUpstreamUnavailable", err)
	}
}

func TestFileService_DeleteLogos(t *testing.T) {
	logo := cdnPrefix + "/abc-logo.png"

	t.Run("program logo is cleared with the value read", func(t *testing.T) {
		d := &fileSvcDeps{programs: &stubProgRepo{getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, LogoURL: &logo}, nil
		}}}
		if err := d.build().DeleteProgramLogo(context.Background(), "p1"); err != nil {
			t.Fatalf("DeleteProgramLogo: %v", err)
		}
		if len(d.files.cleared) != 1 || d.files.cleared[0] != (clearCall{"p1", logo}) {
			t.Fatalf("cleared = %+v", d.files.cleared)
		}
	})

	t.Run("no logo is a no-op", func(t *testing.T) {
		d := &fileSvcDeps{}
		if err := d.build().DeleteProgramLogo(context.Background(), "p1"); err != nil || len(d.files.cleared) != 0 {
			t.Fatalf("err = %v, cleared = %+v", err, d.files.cleared)
		}
	})
}

func TestFileService_DownloadProgramLogo(t *testing.T) {
	t.Run("foreign URL is never fetched", func(t *testing.T) {
		d := &fileSvcDeps{logos: &fakeObjectStore{}}
		_, err := d.build().DownloadProgramLogo(context.Background(), &models.Program{LogoURL: ptr("https://lf.example/logo.png")}, "")
		if !errors.Is(err, domain.ErrFileNotFound) {
			t.Fatalf("err = %v; want ErrFileNotFound", err)
		}
		if len(d.logos.getKeys) != 0 {
			t.Fatal("foreign URL must not be fetched")
		}
	})

	t.Run("own URL reads the escaped key", func(t *testing.T) {
		d := &fileSvcDeps{logos: &fakeObjectStore{object: storedObject("image/png", "")}}
		obj, err := d.build().DownloadProgramLogo(context.Background(), &models.Program{LogoURL: ptr(cdnPrefix + "/abc-my%20logo.png")}, "")
		if err != nil {
			t.Fatalf("DownloadProgramLogo: %v", err)
		}
		if len(d.logos.getKeys) != 1 || d.logos.getKeys[0] != "abc-my logo.png" || obj.CacheControl != "public, max-age=86400" {
			t.Fatalf("get keys = %v, cache = %q", d.logos.getKeys, obj.CacheControl)
		}
	})

	t.Run("non-image is served inert", func(t *testing.T) {
		d := &fileSvcDeps{logos: &fakeObjectStore{object: storedObject("image/svg+xml", "public, max-age=86400")}}
		obj, err := d.build().DownloadProgramLogo(context.Background(), &models.Program{LogoURL: ptr(cdnPrefix + "/abc-logo.svg")}, "")
		if err != nil {
			t.Fatalf("DownloadProgramLogo: %v", err)
		}
		if obj.ContentType != "application/octet-stream" {
			t.Fatalf("content type = %q; want application/octet-stream", obj.ContentType)
		}
	})
}

func TestFileService_UploadTaskFile(t *testing.T) {
	t.Run("assignee stores a private object", func(t *testing.T) {
		d := &fileSvcDeps{
			attachments: &fakeObjectStore{},
			tasks:       &stubTaskAccess{task: &models.Task{ID: "t1", AssigneeID: "mentee", Status: models.TaskStatusInProgress}},
		}
		got, err := d.build().UploadTaskFile(context.Background(), "t1", "mentee", "My Essay.pdf", pdfBytes)
		if err != nil {
			t.Fatalf("UploadTaskFile: %v", err)
		}
		put := d.attachments.puts[0]
		if put.contentType != "application/pdf" || put.cacheControl != "private, no-store" || !strings.HasSuffix(put.key, "-My_Essay.pdf") {
			t.Fatalf("unexpected put %+v", put)
		}
		if got.PublicURL != "" || got.Filename != "My_Essay.pdf" {
			t.Fatalf("unexpected response %+v", got)
		}
		if d.scheduler.entries[0].bucket != domain.ObjectBucketAttachments || d.scheduler.entries[0].locator != put.key {
			t.Fatalf("grace-period entry = %+v", d.scheduler.entries)
		}
		if len(d.files.taskFile) != 1 || d.files.taskFile[0].Next != put.key {
			t.Fatalf("task file column must hold the key: %+v", d.files.taskFile)
		}
	})

	tests := []struct {
		name string
		task *models.Task
		data []byte
		want error
	}{
		{"reviewer cannot upload", &models.Task{AssigneeID: "mentee", Status: models.TaskStatusInProgress}, pdfBytes, domain.ErrForbidden},
		{"complete task is locked", &models.Task{AssigneeID: "actor", Status: models.TaskStatusComplete}, pdfBytes, domain.ErrStateLocked},
		{"image is not a submission type", &models.Task{AssigneeID: "actor", Status: models.TaskStatusSubmitted}, pngBytes, domain.ErrUnsupportedMedia},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &fileSvcDeps{attachments: &fakeObjectStore{}, tasks: &stubTaskAccess{task: tc.task}}
			if _, err := d.build().UploadTaskFile(context.Background(), "t1", "actor", "a.pdf", tc.data); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if len(d.attachments.puts) != 0 || len(d.scheduler.entries) != 0 {
				t.Fatal("rejected upload must not be queued or stored")
			}
		})
	}
}

func TestFileService_DeleteTaskFile(t *testing.T) {
	key := "0b9a3f4e-6d5c-4b3a-9f8e-7d6c5b4a3f2e-essay.pdf"
	tests := []struct {
		name   string
		status models.TaskStatus
		actor  string
		want   error
	}{
		{"before submission", models.TaskStatusInProgress, "mentee", nil},
		{"incomplete", models.TaskStatusIncomplete, "mentee", nil},
		{"submitted keeps its file", models.TaskStatusSubmitted, "mentee", domain.ErrStateLocked},
		{"complete keeps its file", models.TaskStatusComplete, "mentee", domain.ErrStateLocked},
		{"reviewer cannot remove", models.TaskStatusInProgress, "reviewer", domain.ErrForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &fileSvcDeps{tasks: &stubTaskAccess{task: &models.Task{ID: "t1", AssigneeID: "mentee", Status: tc.status, File: &key}}}
			err := d.build().DeleteTaskFile(context.Background(), "t1", tc.actor)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if wantCleared := tc.want == nil; wantCleared != (len(d.files.cleared) == 1) {
				t.Fatalf("cleared = %+v", d.files.cleared)
			}
		})
	}
}

func TestFileService_DownloadTaskFile(t *testing.T) {
	key := "0b9a3f4e-6d5c-4b3a-9f8e-7d6c5b4a3f2e-notes.bat"

	t.Run("serves under the identified type's extension", func(t *testing.T) {
		d := &fileSvcDeps{attachments: &fakeObjectStore{object: storedObject("text/plain; charset=utf-8", "")}, tasks: &stubTaskAccess{task: &models.Task{File: &key}}}
		obj, name, err := d.build().DownloadTaskFile(context.Background(), "t1", "reviewer", "")
		if err != nil {
			t.Fatalf("DownloadTaskFile: %v", err)
		}
		if name != "notes.txt" || obj.CacheControl != "private, no-store" {
			t.Fatalf("name = %q, cache = %q", name, obj.CacheControl)
		}
	})

	t.Run("unknown stored type is served as octet-stream", func(t *testing.T) {
		d := &fileSvcDeps{attachments: &fakeObjectStore{object: storedObject("text/html", "")}, tasks: &stubTaskAccess{task: &models.Task{File: &key}}}
		obj, name, err := d.build().DownloadTaskFile(context.Background(), "t1", "reviewer", "")
		if err != nil {
			t.Fatalf("DownloadTaskFile: %v", err)
		}
		if obj.ContentType != "application/octet-stream" || name != "notes" {
			t.Fatalf("content type = %q, name = %q", obj.ContentType, name)
		}
	})

	t.Run("legacy URL value is never fetched", func(t *testing.T) {
		d := &fileSvcDeps{attachments: &fakeObjectStore{}, tasks: &stubTaskAccess{task: &models.Task{File: ptr("https://legacy.example/x.pdf")}}}
		if _, _, err := d.build().DownloadTaskFile(context.Background(), "t1", "reviewer", ""); !errors.Is(err, domain.ErrFileNotFound) {
			t.Fatalf("err = %v; want ErrFileNotFound", err)
		}
		if len(d.attachments.getKeys) != 0 {
			t.Fatal("URL value must not be fetched")
		}
	})

	t.Run("access check failure propagates", func(t *testing.T) {
		d := &fileSvcDeps{attachments: &fakeObjectStore{}, tasks: &stubTaskAccess{err: domain.ErrForbidden}}
		if _, _, err := d.build().DownloadTaskFile(context.Background(), "t1", "stranger", ""); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("err = %v; want ErrForbidden", err)
		}
	})
}
