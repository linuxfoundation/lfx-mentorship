# Tasks: LFX Mentorship Core Workflow

**Input**: Design documents from `specs/001-mentorship-core-workflow/`

**Prerequisites**: plan.md ✅ | spec.md ✅ | research.md ✅ | data-model.md ✅ | contracts/api.md ✅ | quickstart.md ✅

**No tests requested** — tasks follow spec requirements directly; quickstart.md covers
validation scenarios.

**Organisation**: Tasks grouped by user story to enable independent implementation and
testing. The scaffold (CRUD) already exists; these tasks implement the state-machine
business rules, guards, computed fields, and new endpoints.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no incomplete-task dependency)
- **[Story]**: User story label (US1–US6)
- Exact file paths included in every task

## Path Convention

```text
backend/
├── cmd/mentorship-api/server.go
├── db/migrations/001_initial.up.sql
└── internal/
    ├── domain/
    │   ├── errors.go
    │   ├── repository.go
    │   └── models/
    ├── infrastructure/
    │   ├── auth/
    │   └── db/
    ├── service/
    └── handler/
```

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Error types and notification interface that every user story depends on.

- [x] T001 Add `ErrInvalidStateTransition`, `ErrStateLocked`, and `ErrIneligible` sentinel errors to `backend/internal/domain/errors.go`
- [x] T002 Define `Notifier` interface in `backend/internal/domain/notifier.go` with methods `NotifyMentorDeclined`, `NotifyAdminTasksSubmitted`, `NotifyMenteeAccepted`; add `LogNotifier` stub implementation in `backend/internal/infrastructure/notifier.go`
- [x] T003 [P] Wire `LogNotifier` into `ProgramMemberService`, `ApplicationService`, and `TaskService` constructors in `backend/cmd/mentorship-api/server.go`; update service `New*` signatures to accept `domain.Notifier`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Repository guard queries required by multiple user stories. All must be
complete before any story work begins.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T004 Add `CountOpenTermsByProgram(ctx, programID) (int, error)` to `ProgramTermRepository` interface in `backend/internal/domain/repository.go` and implement it in `backend/internal/infrastructure/db/program_term_repository.go` (`SELECT COUNT(*) … WHERE program_id=$1 AND status='open'`)
- [x] T005 [P] Add `CountBlockingAppsForProgram(ctx, programID string, statuses []string) (int, error)` to `ApplicationRepository` interface and implement in `backend/internal/infrastructure/db/application_repository.go` (used by hide guard)
- [x] T006 [P] Add `CountAcceptedByTerm(ctx, termID string) (int, error)` to `ApplicationRepository` interface and implement in `backend/internal/infrastructure/db/application_repository.go` (used by term close guard)
- [x] T007 [P] Add `FindByTermAndUser(ctx, termID, userID, role string) (*models.Application, error)` to `ApplicationRepository` interface and implement in `backend/internal/infrastructure/db/application_repository.go` (used by reapply guard and duplicate check)
- [x] T008 [P] Add `BulkDeclineByTerm(ctx, termID string) (int64, error)` to `ApplicationRepository` interface and implement in `backend/internal/infrastructure/db/application_repository.go` (`UPDATE … SET status='declined' WHERE program_term_id=$1 AND status='pending'`, returns rows affected)
- [x] T009 [P] Add `CountPrerequisiteTasksByApplication(ctx, applicationID string) (total int, doneCount int, error)` to `TaskRepository` interface and implement in `backend/internal/infrastructure/db/task_repository.go` (`SELECT COUNT(*), COUNT(*) FILTER (WHERE status IN ('submitted','complete')) … WHERE application_id=$1 AND category='prerequisite'`)
- [x] T010 [P] Add `ListPastMenteesByTerm(ctx, termID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)` to `ApplicationRepository` interface and implement in `backend/internal/infrastructure/db/application_repository.go` (WHERE `program_term_id=$1 AND status IN ('graduated','declined','withdrawn')`)
- [x] T011 [P] Add `mapError` cases for `ErrInvalidStateTransition` (→ 409) and `ErrStateLocked` (→ 409) and `ErrIneligible` (→ 422) in `backend/internal/handler/respond.go`

**Checkpoint**: Guard queries complete — user story service logic can begin.

---

## Phase 3: User Story 1 — Program & Term Lifecycle (Priority: P1) 🎯 MVP

**Goal**: Program Admins can take a program through the full lifecycle (draft → published →
hidden → archived), terms enforce the 4-open cap and close guard, and terms return a
computed discovery label.

**Independent Test**: Quickstart Scenarios 1, 2, 6, 9 all pass (program state
transitions, 4-term cap, term close guard, discovery labels).

### Implementation for User Story 1

- [x] T012 [US1] Add `validProgramTransitions` map and state-machine check to `ProgramService.Update` in `backend/internal/service/program_service.go`; return `ErrInvalidStateTransition` for illegal `(from, to)` pairs (see contracts/api.md transition table)
- [x] T013 [US1] Add submission guard to `ProgramService.Update` in `backend/internal/service/program_service.go`: when `to = submitted`, verify all required fields (name, description, repo_link, logo_url) are non-empty and `CountOpenTermsByProgram ≥ 1`; return `ErrInvalidInput` with message on failure
- [x] T014 [US1] Add hide guard to `ProgramService.Update` in `backend/internal/service/program_service.go`: when `to = hidden`, call `CountBlockingAppsForProgram` with statuses `{pending, accepted, graduated, hold}`; return `ErrStateLocked` if count > 0
- [x] T015 [P] [US1] Add `ProgramStatusHidden` already exists — verify it is handled in all switch statements in `backend/internal/domain/models/program.go`; add `deleted` constant and `closed`/`deleted` handling to `ProgramTermService.Update` in `backend/internal/service/program_term_service.go` (accept `'deleted'` as valid status)
- [x] T016 [US1] Add open-term cap guard to `ProgramTermService.Create` in `backend/internal/service/program_term_service.go`: call `CountOpenTermsByProgram`; if ≥ 4, return `ErrInvalidInput` with message `"a program may not have more than 4 open terms"`
- [x] T017 [US1] Add close guard to `ProgramTermService.Update` in `backend/internal/service/program_term_service.go`: when `to = closed`, call `CountAcceptedByTerm`; if > 0, return `ErrStateLocked`; add reopen guard: when `to = open` and `end_date_time < now`, return `ErrStateLocked`
- [x] T018 [P] [US1] Add `DiscoveryLabel(now time.Time) string` method on `ProgramTerm` struct in `backend/internal/domain/models/program_term.go`; implement the four-state mapping from data-model.md
- [x] T019 [P] [US1] Extend `ProgramTerm` JSON response struct (or inline) to include `discovery_label` computed from `DiscoveryLabel(time.Now())` in `backend/internal/handler/program_term_handler.go` for both single-term and list responses

**Checkpoint**: Program lifecycle and term guards fully functional. Scenarios 1, 2, 6, 9 pass.

---

## Phase 4: User Story 2 — Mentor Invitation & Self-Request (Priority: P2)

**Goal**: Program Admins can invite mentors via tokenised email links; mentors can
self-request; all transitions are tracked correctly.

**Independent Test**: Quickstart Scenario 3 passes (invite, accept, decline, self-request approve).

### Implementation for User Story 2

- [x] T020 [US2] Create `backend/internal/infrastructure/auth/invite_token.go`: `GenerateInviteToken(programID, userID string, ttl time.Duration) (string, error)` and `ValidateInviteToken(token, secret string) (programID, userID string, error)` using the existing JWT library
- [x] T021 [US2] Update `ProgramMemberService.Create` in `backend/internal/service/program_member_service.go` to call `GenerateInviteToken` and `Notifier.NotifyMentorInvited` when `status = invited`; accept `requested` as a valid initial status for self-requests
- [x] T022 [US2] Add decline notification call in `ProgramMemberService.Update` in `backend/internal/service/program_member_service.go`: when `to = declined` (from `invited` or `requested`), call `Notifier.NotifyMentorDeclined`
- [x] T023 [US2] Create `backend/internal/handler/mentor_invite_handler.go` with `MentorInviteHandler` struct and `Accept(w, r)` / `Decline(w, r)` methods; parse JWT token from path param, call `ProgramMemberService.AcceptInvite` / `DeclineInvite`
- [x] T024 [US2] Add `AcceptInvite(ctx, token string)` and `DeclineInvite(ctx, token string)` to `ProgramMemberService` in `backend/internal/service/program_member_service.go`
- [x] T025 [US2] Register routes `POST /v1/mentor-invites/{token}/accept` and `POST /v1/mentor-invites/{token}/decline` as **public** (no auth middleware) in `backend/cmd/mentorship-api/server.go`

**Checkpoint**: Invite flow end-to-end functional. Scenario 3 passes.

---

## Phase 5: User Story 3 — Mentee Profile & Application (Priority: P2)

**Goal**: Mentees face an eligibility gate before profile creation; applications are
gated by the term's application window; reapply rules are enforced.

**Independent Test**: Quickstart Scenarios 4a, 4b, and reapply from declined (→ 422) pass.

### Implementation for User Story 3

- [x] T026 [US3] Add `UserProfileRepository.CountActiveApprenticeProfiles(ctx, userID string) (int, error)` to repository interface and implement in `backend/internal/infrastructure/db/user_profile_repository.go` (`SELECT COUNT(*) … WHERE user_id=$1 AND profile_type='apprentice'`)
- [x] T027 [US3] Add eligibility gate to `UserProfileService.Create` in `backend/internal/service/user_profile_service.go`: when `profile_type = apprentice`, call `CountActiveApprenticeProfiles`; if > 0, return `ErrIneligible` with message `"an active apprentice profile already exists"` (age and work-eligibility criteria are client-side confirmation gates passed in as booleans on the create input)
- [x] T028 [P] [US3] Add `AgeEligible` and `WorkEligible` boolean fields to `UserProfileCreateInput` in `backend/internal/domain/models/user_profile.go`; validate both must be `true` in `UserProfileService.Create` when `profile_type = apprentice`
- [x] T029 [US3] Add application window guard to `ApplicationService.Create` in `backend/internal/service/application_service.go`: fetch the `ProgramTerm` by ID, verify `status = open` and `now` is within `[application_start_date, application_end_date]`; return `ErrInvalidInput` on failure
- [x] T030 [US3] Add duplicate/reapply guard to `ApplicationService.Create` in `backend/internal/service/application_service.go`: call `FindByTermAndUser`; if existing record has `status = declined`, return `ErrInvalidInput` `"reapplication from a declined application is not allowed"`; if `status = withdrawn` and window is open, allow the new application
- [x] T031 [US3] Add prerequisite task cloning to `ApplicationService.Create` in `backend/internal/service/application_service.go`: after creating the application, load `programs.task_templates` JSONB via program repo, bulk-insert one `Task` per template entry with `category = prerequisite`, `application_id = newApp.ID`, `status = incomplete`

**Checkpoint**: Mentee application flow fully guarded. Scenarios 4a and 4b pass.

---

## Phase 6: User Story 4 — Prerequisite Task Evaluation (Priority: P2)

**Goal**: Task status transitions are actor-gated; completing all prerequisite tasks
sets `tasks_submitted = true` and notifies the admin.

**Independent Test**: Quickstart Scenarios 4c and 4d pass.

### Implementation for User Story 4

- [x] T032 [US4] Add caller-role resolution to `TaskService.Update` in `backend/internal/service/task_service.go`: extract caller's user ID from context, look up `program_members` to determine if caller is `assignee` (mentee) or `program_admin/mentor`; attach resolved role to the update call
- [x] T033 [US4] Add actor-permission check to `TaskService.Update` in `backend/internal/service/task_service.go`: mentee callers may only set `in_progress` (from `incomplete`) or `submitted` (from `in_progress`); program_admin/mentor callers may set `complete` or reset to `incomplete`; all others return `ErrForbidden`
- [x] T034 [US4] Add `tasks_submitted` side-effect to `TaskService.Update` in `backend/internal/service/task_service.go`: after a successful update to `submitted` or `complete`, if the task has a non-nil `application_id` and `category = prerequisite`, call `CountPrerequisiteTasksByApplication`; if `doneCount == total`, set `applications.tasks_submitted = true` via `ApplicationRepository.Update` and call `Notifier.NotifyAdminTasksSubmitted`
- [x] T035 [P] [US4] Add `ErrForbidden` sentinel to `backend/internal/domain/errors.go` and map it to HTTP 403 in `backend/internal/handler/respond.go`

**Checkpoint**: Task actor permissions and tasks_submitted trigger working. Scenarios 4c and 4d pass.

---

## Phase 7: User Story 5 — Application Disposition (Priority: P1)

**Goal**: Full application state machine; `accepted` requires `attendance_type`;
notifications fire on accept.

**Independent Test**: Quickstart Scenarios 4e and 4f pass (accept requires
attendance_type, graduate flow works).

### Implementation for User Story 5

- [x] T036 [US5] Add full state-machine check to `ApplicationService.Update` in `backend/internal/service/application_service.go`: fetch current application, enforce valid `(from, to)` transitions per data-model.md lifecycle; return `ErrInvalidStateTransition` for illegal pairs
- [x] T037 [US5] Add `attendance_type` required guard to `ApplicationService.Update` in `backend/internal/service/application_service.go`: when `to = accepted` and `input.AttendanceType == nil`, return `ErrInvalidInput` `"attendance_type is required when accepting an application"`
- [x] T038 [US5] Add notification side-effect to `ApplicationService.Update` in `backend/internal/service/application_service.go`: when `to = accepted`, call `Notifier.NotifyMenteeAccepted` with application and attendance details
- [x] T039 [US5] Add `withdrawn` actor guard to `ApplicationHandler.Update` in `backend/internal/handler/application_handler.go`: extract caller user ID from context; if `input.Status = "withdrawn"` and caller is not the application's `user_id`, return 403

**Checkpoint**: Full disposition state machine enforced. Scenarios 4e, 4f, and 5 (hide guard) all pass.

---

## Phase 8: User Story 6 — Supporting Operations (Priority: P3)

**Goal**: Bulk decline, CSV export, and past mentees view are available to program admins.

**Independent Test**: Quickstart Scenarios 7, 8, 9 (partial — discovery label is US1)
and Past Mentees view pass.

### Implementation for User Story 6

- [x] T040 [US6] Add `BulkDecline(ctx, termID string) (int64, error)` to `ApplicationService` in `backend/internal/service/application_service.go`; call `ApplicationRepository.BulkDeclineByTerm`; return count
- [x] T041 [P] [US6] Add `BulkDecline(w, r)` handler method to `ApplicationHandler` in `backend/internal/handler/application_handler.go`; respond with `{"declined_count": N}`
- [x] T042 [US6] Add `ExportByTerm(w, r)` handler method to `ApplicationHandler` in `backend/internal/handler/application_handler.go`: set `Content-Type: text/csv`, stream CSV rows from `ApplicationRepository.ListByProgramTerm` cursor using `encoding/csv`; support `?status=`, `?tasks_submitted=`, `?role=` query params
- [x] T043 [P] [US6] Add `ListPastMentees(ctx, termID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)` to `ApplicationService` in `backend/internal/service/application_service.go`; delegate to `ApplicationRepository.ListPastMenteesByTerm`
- [x] T044 [P] [US6] Add `PastMentees(w, r)` handler method to `ApplicationHandler` in `backend/internal/handler/application_handler.go`
- [x] T045 [US6] Register new routes in `backend/cmd/mentorship-api/server.go`:
  - `POST /v1/program-terms/{id}/applications/bulk-decline` (auth)
  - `GET /v1/program-terms/{id}/applications/export` (auth)
  - `GET /v1/program-terms/{id}/past-mentees` (public)

**Checkpoint**: All supporting operations accessible. Scenarios 7 and 8 pass.

---

## Phase 9: Polish & Cross-Cutting Concerns

- [x] T046 [P] Verify caller identity (LFID / user UUID) is extracted from JWT claims and stored in request context in `backend/internal/infrastructure/auth/` middleware; ensure all handler methods that need actor checks can read it
- [x] T047 [P] Run `go build ./...` from `backend/` and resolve any compilation errors introduced by interface additions and new method signatures
- [x] T048 [P] Run `go vet ./...` from `backend/` and resolve any warnings
- [x] T049 Run quickstart.md scenarios 1–9 against local dev server and record pass/fail; file issues for any failing scenarios

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — BLOCKS all user stories
- **Phase 3–8 (User Stories)**: All depend on Phase 2; can proceed in priority order
  - Phase 3 (US1) and Phase 7 (US5) are P1 — implement first
  - Phase 4 (US2), Phase 5 (US3), Phase 6 (US4) are P2 — can run in parallel after Phase 2
  - Phase 8 (US6) is P3 — after Phase 5 and Phase 7 complete
- **Phase 9 (Polish)**: After all desired stories complete

### User Story Dependencies

| Story | Depends on | Can start after |
|-------|-----------|----------------|
| US1 — Program & Term Lifecycle | Phase 2 | T011 complete |
| US2 — Mentor Invitation | Phase 2 | T011 complete |
| US3 — Mentee Profile & Application | Phase 2, US1 (term validation) | T017 complete |
| US4 — Task Evaluation | Phase 2, US3 (application exists) | T031 complete |
| US5 — Application Disposition | Phase 2, US3 | T031 complete |
| US6 — Supporting Operations | Phase 2, US5 (state machine) | T039 complete |

### Within Each User Story

- Repository queries (Phase 2) before service logic
- Service logic before handler additions
- Handler additions before route registration

### Parallel Opportunities

- T005, T006, T007, T008, T009, T010, T011 can all run in parallel (different files)
- T015, T018, T019 within US1 can run in parallel
- US2 (T020–T025) and US3 (T026–T031) can run in parallel after Phase 2
- US4 (T032–T035) and US5 (T036–T039) can run in parallel after US3
- T041, T043, T044 within US6 can run in parallel
- T046, T047, T048 in Polish can all run in parallel

---

## Implementation Strategy

**MVP** (deliver first): Phase 1 + Phase 2 + Phase 3 (US1) + Phase 7 (US5)

This gives a fully working program lifecycle with application acceptance — the core
program_admin journey is testable end-to-end after these 4 phases (tasks T001–T019,
T036–T039).

**Increment 2**: Phase 4 (US2) + Phase 5 (US3) + Phase 6 (US4)

Adds the full mentee journey: eligibility gate, application window, task evaluation.

**Increment 3**: Phase 8 (US6) + Phase 9

Adds operational tooling (bulk decline, CSV export, past mentees) and final polish.

---

## Task Summary

| Phase | Tasks | Stories |
|-------|-------|---------|
| 1 — Setup | T001–T003 | — |
| 2 — Foundational | T004–T011 | — |
| 3 — US1 Program & Term Lifecycle | T012–T019 | US1 |
| 4 — US2 Mentor Invitation | T020–T025 | US2 |
| 5 — US3 Mentee Profile & Application | T026–T031 | US3 |
| 6 — US4 Task Evaluation | T032–T035 | US4 |
| 7 — US5 Application Disposition | T036–T039 | US5 |
| 8 — US6 Supporting Operations | T040–T045 | US6 |
| 9 — Polish | T046–T049 | — |
| **Total** | **49 tasks** | **6 stories** |
