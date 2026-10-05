// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var fileSvcTracer = otel.Tracer("files-service")

const (
	// MaxLogoBytes caps image uploads, under the platform's 20 MB ceiling.
	MaxLogoBytes = 2 << 20
	// MaxTaskFileBytes is the platform per-file ceiling.
	MaxTaskFileBytes = 20 << 20

	// uploadGracePeriod must outlast the gateway's 120s upload timeout, so a new
	// object is never reclaimed while its upload can still commit.
	uploadGracePeriod = 15 * time.Minute

	// publicCacheControl is short-lived: a CDN copy must converge within a day of a delete.
	publicCacheControl  = "public, max-age=86400"
	privateCacheControl = "private, no-store"
	octetStream         = "application/octet-stream"
)

var errStorageNotConfigured = fmt.Errorf("%w: object storage is not configured", domain.ErrUpstreamUnavailable)

// LogoBucket is the CDN-fronted public bucket that program and profile logos live in.
type LogoBucket struct {
	Store domain.ObjectStore
	// CDNURLPrefix is the browser-reachable base URL stored logo URLs start with, without a trailing slash.
	CDNURLPrefix string
}

func (b *LogoBucket) publicURL(key string) string {
	return b.CDNURLPrefix + "/" + url.PathEscape(key)
}

type taskAccess interface {
	GetByIDForActor(ctx context.Context, id, actorID string) (*models.Task, error)
}

type deletionScheduler interface {
	Schedule(ctx context.Context, bucket domain.ObjectBucket, locator string, delay time.Duration) (string, error)
}

// FileService implements the upload, download and removal use cases for program logos,
// profile logos and task submissions. Logo downloads are a fallback to the CDN.
//
// Every upload writes a fresh key and never overwrites, and every dropped locator goes
// through the object_deletions queue rather than an inline delete.
type FileService struct {
	files     domain.FileRepository
	deletions deletionScheduler
	programs  domain.ProgramRepository
	profiles  domain.UserProfileRepository
	tasks     taskAccess
	// logos and attachments are nil when their bucket is not configured.
	logos       *LogoBucket
	attachments domain.ObjectStore
}

// NewFileService returns a FileService. Pass a nil logos or attachments to disable that file class.
func NewFileService(
	files domain.FileRepository,
	deletions deletionScheduler,
	programs domain.ProgramRepository,
	profiles domain.UserProfileRepository,
	tasks taskAccess,
	logos *LogoBucket,
	attachments domain.ObjectStore,
) *FileService {
	return &FileService{files: files, deletions: deletions, programs: programs, profiles: profiles, tasks: tasks, logos: logos, attachments: attachments}
}

// UploadProgramLogo stores a new logo for the program and points programs.logo_url at it.
func (s *FileService) UploadProgramLogo(ctx context.Context, programID string, data []byte) (*models.UploadedFile, error) {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.UploadProgramLogo")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID))

	if s.logos == nil {
		return nil, errStorageNotConfigured
	}
	program, err := s.programs.GetByID(ctx, programID)
	if err != nil {
		return nil, fmt.Errorf("get program for logo upload: %w", err)
	}
	if err := programLogoEditable(program); err != nil {
		return nil, err
	}
	uploaded, err := s.replaceLogo(ctx, data, func(rep domain.FileReplacement) error {
		rep.RowID, rep.Previous = program.ID, program.LogoURL
		return s.files.ReplaceProgramLogo(ctx, rep)
	})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return uploaded, nil
}

// DeleteProgramLogo clears the program's logo and queues the object for deletion.
func (s *FileService) DeleteProgramLogo(ctx context.Context, programID string) error {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.DeleteProgramLogo")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID))

	program, err := s.programs.GetByID(ctx, programID)
	if err != nil {
		return fmt.Errorf("get program for logo removal: %w", err)
	}
	if err := programLogoEditable(program); err != nil {
		return err
	}
	if program.LogoURL == nil || *program.LogoURL == "" {
		return nil
	}
	if err := s.files.ClearProgramLogo(ctx, program.ID, *program.LogoURL); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

// DownloadProgramLogo opens the logo of a program the caller has already resolved as visible.
func (s *FileService) DownloadProgramLogo(ctx context.Context, program *models.Program, byteRange string) (*domain.StoredObject, error) {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.DownloadProgramLogo")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", program.ID))

	if s.logos == nil {
		return nil, errStorageNotConfigured
	}
	return s.getLogo(ctx, program.LogoURL, byteRange)
}

// UploadProfileLogo stores a new logo for one of the actor's own profiles.
func (s *FileService) UploadProfileLogo(ctx context.Context, profileID, actorID string, data []byte) (*models.UploadedFile, error) {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.UploadProfileLogo")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))

	if s.logos == nil {
		return nil, errStorageNotConfigured
	}
	profile, err := s.ownProfile(ctx, profileID, actorID)
	if err != nil {
		return nil, err
	}
	uploaded, err := s.replaceLogo(ctx, data, func(rep domain.FileReplacement) error {
		rep.RowID, rep.Previous = profile.ID, profile.LogoURL
		return s.files.ReplaceProfileLogo(ctx, rep)
	})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return uploaded, nil
}

// DeleteProfileLogo clears one of the actor's profile logos and queues the object for deletion.
func (s *FileService) DeleteProfileLogo(ctx context.Context, profileID, actorID string) error {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.DeleteProfileLogo")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))

	profile, err := s.ownProfile(ctx, profileID, actorID)
	if err != nil {
		return err
	}
	if profile.LogoURL == nil || *profile.LogoURL == "" {
		return nil
	}
	if err := s.files.ClearProfileLogo(ctx, profile.ID, *profile.LogoURL); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

// DownloadProfileLogo opens a profile's logo; only profiles in the public directory are served.
func (s *FileService) DownloadProfileLogo(ctx context.Context, profileID, byteRange string) (*domain.StoredObject, error) {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.DownloadProfileLogo")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))

	if s.logos == nil {
		return nil, errStorageNotConfigured
	}
	listed, err := s.files.IsProfilePubliclyListed(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if !listed {
		return nil, domain.ErrUserProfileNotFound
	}
	profile, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile for logo download: %w", err)
	}
	return s.getLogo(ctx, profile.LogoURL, byteRange)
}

// UploadTaskFile stores the assignee's submission for a task that is not yet complete.
func (s *FileService) UploadTaskFile(ctx context.Context, taskID, actorID, filename string, data []byte) (*models.UploadedFile, error) {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.UploadTaskFile")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", taskID))

	if s.attachments == nil {
		return nil, errStorageNotConfigured
	}
	task, err := s.assignedTask(ctx, taskID, actorID)
	if err != nil {
		return nil, err
	}
	if task.Status == models.TaskStatusComplete {
		return nil, fmt.Errorf("%w: a completed task's file cannot change", domain.ErrStateLocked)
	}
	t, err := identifyUpload(data, MaxTaskFileBytes, taskFileTypes, "PDF, DOC, DOCX or plain text")
	if err != nil {
		return nil, err
	}
	key := newObjectKey(filename)
	err = s.replace(ctx, domain.ObjectBucketAttachments, key,
		func() error { return s.attachments.PutObject(ctx, key, data, t.contentType, privateCacheControl) },
		func(rep domain.FileReplacement) error {
			rep.RowID, rep.Previous = task.ID, task.File
			return s.files.ReplaceTaskFile(ctx, rep)
		})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return &models.UploadedFile{
		Filename:    downloadFilename(key, t),
		ContentType: t.contentType,
		Size:        int64(len(data)),
	}, nil
}

// DeleteTaskFile removes the assignee's submission before the task is submitted. A
// submitted file is replaced through the upload route instead, so a task that requires
// a file always keeps one.
func (s *FileService) DeleteTaskFile(ctx context.Context, taskID, actorID string) error {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.DeleteTaskFile")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", taskID))

	task, err := s.assignedTask(ctx, taskID, actorID)
	if err != nil {
		return err
	}
	if task.Status != models.TaskStatusIncomplete && task.Status != models.TaskStatusInProgress {
		return fmt.Errorf("%w: a %s task's file can only be replaced", domain.ErrStateLocked, task.Status)
	}
	if task.File == nil || *task.File == "" {
		return nil
	}
	if err := s.files.ClearTaskFile(ctx, task.ID, *task.File); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

// DownloadTaskFile opens a task's submission for its assignee or a reviewer, returning
// the object and the attachment filename to serve it under.
func (s *FileService) DownloadTaskFile(ctx context.Context, taskID, actorID, byteRange string) (*domain.StoredObject, string, error) {
	ctx, span := fileSvcTracer.Start(ctx, "FileService.DownloadTaskFile")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", taskID))

	if s.attachments == nil {
		return nil, "", errStorageNotConfigured
	}
	task, err := s.tasks.GetByIDForActor(ctx, taskID, actorID)
	if err != nil {
		return nil, "", err
	}
	if task.File == nil {
		return nil, "", domain.ErrFileNotFound
	}
	key, ok := domain.ObjectKeyFromLocator(domain.ObjectBucketAttachments, *task.File, "")
	if !ok {
		return nil, "", domain.ErrFileNotFound
	}
	obj, err := s.attachments.GetObject(ctx, key, byteRange)
	if err != nil {
		return nil, "", err
	}
	t, known := fileTypeByContentType(obj.ContentType)
	if !known {
		t = fileType{contentType: octetStream}
	}
	obj.ContentType = t.contentType
	obj.CacheControl = privateCacheControl
	return obj, downloadFilename(key, t), nil
}

// replace runs the three-step replace: queue the new locator for deletion after a grace
// period and commit, write the object, then let write swap the row, cancel that entry and
// queue the previous locator in one transaction. If write never commits, the grace-period
// entry reclaims the orphaned object.
func (s *FileService) replace(ctx context.Context, bucket domain.ObjectBucket, locator string, put func() error, write func(domain.FileReplacement) error) error {
	pendingID, err := s.deletions.Schedule(ctx, bucket, locator, uploadGracePeriod)
	if err != nil {
		return err
	}
	if err := put(); err != nil {
		return err
	}
	return write(domain.FileReplacement{Next: locator, PendingDeletionID: pendingID})
}

func (s *FileService) replaceLogo(ctx context.Context, data []byte, write func(domain.FileReplacement) error) (*models.UploadedFile, error) {
	t, err := identifyUpload(data, MaxLogoBytes, logoFileTypes, "PNG or JPEG")
	if err != nil {
		return nil, err
	}
	filename := "logo" + t.extension
	key := newObjectKey(filename)
	publicURL := s.logos.publicURL(key)
	err = s.replace(ctx, domain.ObjectBucketLogos, publicURL,
		func() error { return s.logos.Store.PutObject(ctx, key, data, t.contentType, publicCacheControl) },
		write)
	if err != nil {
		return nil, err
	}
	return &models.UploadedFile{
		PublicURL:   publicURL,
		Filename:    filename,
		ContentType: t.contentType,
		Size:        int64(len(data)),
	}, nil
}

func (s *FileService) getLogo(ctx context.Context, stored *string, byteRange string) (*domain.StoredObject, error) {
	if stored == nil {
		return nil, domain.ErrFileNotFound
	}
	key, ok := domain.ObjectKeyFromLocator(domain.ObjectBucketLogos, *stored, s.logos.CDNURLPrefix)
	if !ok {
		return nil, domain.ErrFileNotFound
	}
	obj, err := s.logos.Store.GetObject(ctx, key, byteRange)
	if err != nil {
		return nil, err
	}
	// Anything but an allowlisted image is served inert, so a legacy SVG never renders on the API origin.
	if t, known := fileTypeByContentType(obj.ContentType); !known || !t.in(logoFileTypes) {
		obj.ContentType = octetStream
	}
	if obj.CacheControl == "" {
		obj.CacheControl = publicCacheControl
	}
	return obj, nil
}

// programLogoEditable locks the logo of an archived program, a terminal state. A rejected
// program stays editable, since resubmitting it requires a logo.
func programLogoEditable(program *models.Program) error {
	if program.Status == models.ProgramStatusArchived {
		return fmt.Errorf("%w: an archived program's logo cannot change", domain.ErrStateLocked)
	}
	return nil
}

func (s *FileService) ownProfile(ctx context.Context, profileID, actorID string) (*models.UserProfile, error) {
	profile, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if actorID == "" || profile.UserID != actorID {
		return nil, fmt.Errorf("%w: profile belongs to another user", domain.ErrForbidden)
	}
	return profile, nil
}

func (s *FileService) assignedTask(ctx context.Context, taskID, actorID string) (*models.Task, error) {
	task, err := s.tasks.GetByIDForActor(ctx, taskID, actorID)
	if err != nil {
		return nil, err
	}
	if task.AssigneeID != actorID {
		return nil, fmt.Errorf("%w: only the task assignee may change its file", domain.ErrForbidden)
	}
	return task, nil
}

// reservedFileField rejects a file column on a generic route; only the file routes write them.
func reservedFileField(field string, value *string) error {
	if value == nil {
		return nil
	}
	return fmt.Errorf("%w: %s is set only through its file route", domain.ErrInvalidInput, field)
}

// identifyUpload enforces the size cap and the allowlist on the payload bytes.
func identifyUpload(data []byte, maxBytes int, allowed []fileType, allowedNames string) (fileType, error) {
	if len(data) == 0 {
		return fileType{}, fmt.Errorf("%w: file is empty", domain.ErrInvalidInput)
	}
	if len(data) > maxBytes {
		return fileType{}, fmt.Errorf("%w: file exceeds %d MB", domain.ErrPayloadTooLarge, maxBytes>>20)
	}
	t, ok := identifyFileType(data, allowed)
	if !ok {
		return fileType{}, fmt.Errorf("%w: file must be %s", domain.ErrUnsupportedMedia, allowedNames)
	}
	return t, nil
}
