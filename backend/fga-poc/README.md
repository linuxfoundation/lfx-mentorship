# Mentorship FGA — Authorization Model

Fine-grained authorization model for the LFX Mentorship service. Prototyped
and validated here before being integrated into the LFX Platform v2 production
stack.

---

## Model overview

Four types, each scoped to its parent via a relation:

```
project:<slug>
  └── program:<id>
        └── application:<id>
              └── task:<id>
```

| Type | Key relations | Key permissions |
|---|---|---|
| `project` | `super_admin`, `program_approver`, `program_admin` | `can_create_program`, `can_approve_program` |
| `program` | `program_admin`, `mentor` (+ inherited from `project`) | `can_view`, `can_invite_mentor`, `can_create_term`, `can_add_prerequisite_task`, `can_add_task`, `can_view_applications`, `can_decide_application`, `can_approve` |
| `application` | `mentee` (+ inherited from `program`) | `can_view`, `can_decide`, `can_add_task` |
| `task` | `assignee` (+ inherited from `application`) | `can_view`, `can_update_status` |

### Role summary

| Role | Scope | What they can do |
|---|---|---|
| `super_admin` | per-project | everything within that project |
| `program_approver` | per-project | view + approve programs in their project, view applications |
| `program_admin` | per-project + per-program | create programs (project-level); full control of their specific program and its applications/tasks |
| `mentor` | per-program | view program + applications, accept/reject applications, create terms, add prerequisite and non-prerequisite tasks, update task status — cannot invite mentors |
| `mentee` | per-application | view their own application and tasks |

---


### Load model and tuples

```bash
FGA_STORE_ID=$(fga store create --name "mentorship-fga" --api-url http://localhost:8080 \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['store']['id'])")

fga model write --store-id="$FGA_STORE_ID" --api-url http://localhost:8080 --file model.fga
fga tuple import --store-id="$FGA_STORE_ID" --api-url http://localhost:8080 --file tuples.yaml

echo "Playground: http://localhost:9000/playground"
```

### Run the test suite

```bash
fga model test --tests tests.yaml
# Expected: Tests 13/13 passing, Checks 77/77 passing
```

### Teardown

```bash
docker stop openfga openfga-caddy
docker rm openfga openfga-caddy
```

---

## Integration into LFX Platform v2

The production path requires three PRs across three repos:

---

### PR 1 — `linuxfoundation/lfx-v2-helm`

**What:** Add the mentorship types to the shared OpenFGA authorization model.

**File:** `charts/lfx-platform/files/model.fga`

Append the `project`, `program`, `application`, and `task` types from `model.fga` in this
repo. The `platform` type in lfx-v2-helm can be removed or left as a stub if other services
still reference it — do not let it conflict with the new `project` root.

**File:** `charts/lfx-platform/templates/openfga/model.yaml`

Bump the version. Adding types is a **major** bump per the file's own versioning
guidelines (currently `14.3.0` → `15.0.0`):

```yaml
- version:
    major: 15
    minor: 0
    patch: 0
```

**PR title:** `feat(fga): add mentorship types (project, program, application, task)`

---

### PR 2 — `linuxfoundation/lfx-v2-fga-sync`

**What:** Register the mentorship service in the protected-types inventory.

**File:** `docs/fga-protected-types.md`

Add a row to the Services table:

```markdown
| lfx-v2-mentorship-service | project, program, application, task | fga-contract.md |
```

Add a section under **FGA Object Types by Domain**:

```markdown
### Mentorship: `lfx-v2-mentorship-service`

FGA contract: docs/fga-contract.md in lfx-v2-mentorship-service

| Object type   | Operations |
|---|---|
| `project`     | update_access, delete_access, member_put, member_remove |
| `program`     | update_access, delete_access, member_put, member_remove |
| `application` | update_access, delete_access, member_put, member_remove |
| `task`        | update_access, delete_access, member_put, member_remove |
```

**PR title:** `docs: add lfx-v2-mentorship-service to FGA protected-types inventory`

---

### PR 3 — `linuxfoundation/lfx-v2-mentorship-service`

**What:** Three things — a contract doc, NATS publishers, and access check calls.

#### 3a. Create `docs/fga-contract.md`

Document the object types, which lifecycle events map to which NATS subject, and
the tuple shape for each. Use an existing service (e.g.
`lfx-v2-committee-service/docs/fga-contract.md`) as a shape reference only — do
not copy relations or business logic.

#### 3b. NATS publishers

Publish to the generic `lfx.fga-sync.*` subjects consumed by `lfx-v2-fga-sync`.
See [docs/client-guide.md](https://github.com/linuxfoundation/lfx-v2-fga-sync/blob/main/docs/client-guide.md)
for the authoritative envelope format.

**Program events:**

| Lifecycle event | Subject | Tuples written |
|---|---|---|
| Program created | `lfx.fga-sync.update_access` | `project` link, `program_admin`, initial `mentor` if any |
| Program deleted | `lfx.fga-sync.delete_access` | all tuples removed |
| Mentor invited/accepted | `lfx.fga-sync.member_put` | `mentor` relation on the program |
| Mentor removed | `lfx.fga-sync.member_remove` | `mentor` relation removed |

**Application events:**

| Lifecycle event | Subject | Tuples written |
|---|---|---|
| Application submitted | `lfx.fga-sync.update_access` | `program` link, `mentee` relation |
| Application withdrawn/deleted | `lfx.fga-sync.delete_access` | all tuples removed |

**Task events:**

| Lifecycle event | Subject | Tuples written |
|---|---|---|
| Task created | `lfx.fga-sync.update_access` | `application` link, `assignee` relation |
| Task deleted | `lfx.fga-sync.delete_access` | all tuples removed |

#### 3c. Replace in-service permission checks with FGA access checks

For each place the mentorship service currently enforces a permission (e.g.
"is this user allowed to decide this application?"), replace it with a NATS
request/reply call to `lfx.access_check.request`.

Map each existing check to the corresponding FGA relation:

| Old check | FGA call |
|---|---|
| is user a program admin? | `can_invite_mentor` on `program:<id>` |
| can user approve a program? | `can_approve` on `program:<id>` |
| can user decide this application? | `can_decide` on `application:<id>` |
| can user update this task? | `can_update_status` on `task:<id>` |

**PR title:** `feat(fga): implement FGA authorization for programs, applications, and tasks`

---

## Test coverage

All 13 test scenarios from `tests.yaml` must pass against the merged model
before the lfx-v2-helm PR is merged. After deploying to staging, replay a
representative set of NATS events and verify the results against the assertions
in `tests.yaml` using `lfx.access_check.request`.

| Test | Key assertion |
|---|---|
| project-level program creation and approval rights | `carla` can create, `priya` can approve, `aisha` can do both |
| super_admin has unrestricted access to every object | `aisha` allowed on all types at all levels |
| program_admin has full control over their program | `carla` on `go-2026`: all ops allowed |
| program_admin has no access to other programs | `carla` on `rust-2026`: all denied |
| mentor can view their program and applications but cannot manage or decide | `miguel` on `go-2026`: `can_view` + `can_view_applications` yes; `can_invite_mentor`, `can_decide_application` denied |
| mentor has zero access to programs they weren't invited to | `miguel` on `rust-2026`: all denied |
| mentee can view only their own application | `mia` on `app-mia`: allowed; on `app-noah`: denied |
| program_admin and mentor can both decide applications | `carla` + `miguel` on `app-mia`: both `can_decide` yes |
| mentor can add both prerequisite and non-prerequisite tasks | `miguel` on `program:go-2026`: `can_add_task` yes, `can_add_prerequisite_task` yes |
| mentor can add tasks to applications and update task status | `miguel` on `app-mia`: `can_add_task` yes; on `task-mia-1`: `can_update_status` yes |
| mentee can view/update only their own tasks | `mia` on `task-mia-1`: allowed; `noah`: denied |
| program_admin and mentor can view and update tasks in their program | `carla` + `miguel` on `task-mia-1`: both allowed |
| program_approver can view and approve but not manage | `priya`: `can_view` + `can_approve` yes; ops denied |
| program_approver can see applications but not decide | `priya` on `app-mia`: `can_view` yes, `can_decide` no |
