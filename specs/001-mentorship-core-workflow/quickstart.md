# Quickstart Validation Guide: LFX Mentorship Core Workflow

**Phase**: 1 | **Date**: 2026-08-20 | **Plan**: [plan.md](plan.md)

End-to-end validation scenarios proving the feature works. Does not include
implementation code — see `tasks.md` and the implementation phase for that.

---

## Prerequisites

```bash
# Start PostgreSQL (Docker)
docker compose up -d db

# Apply migrations
cd backend
go run ./cmd/mentorship-migrate/  # or: psql -f db/migrations/001_initial.up.sql

# Start the API with mock auth bypass
DATABASE_DSN="host=127.0.0.1 port=5433 dbname=mentorship user=mentorship password=mentorship sslmode=disable" \
DISABLED_MOCK_LOCAL_PRINCIPAL=localdev \
ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS=true \
PORT=8090 \
go run ./cmd/mentorship-api/
```

All `curl` examples below assume `BASE=http://localhost:8090/v1` and
`AUTH="-H 'X-Mock-Principal: localdev'"`.

---

## Scenario 1 — Program Lifecycle (happy path)

### 1a. Create a draft program

```bash
curl -s -X POST $BASE/programs $AUTH \
  -H 'Content-Type: application/json' \
  -d '{"name":"Go Mentorship 2026","slug":"go-2026","description":"Learn Go.","repo_link":"https://github.com/example/go","logo_url":"https://example.com/logo.png"}'
# Expected: 201, body contains "status":"draft"
```

### 1b. Add a term (required before submission)

```bash
PROGRAM_ID=<id from 1a>
curl -s -X POST $BASE/programs/$PROGRAM_ID/terms $AUTH \
  -H 'Content-Type: application/json' \
  -d '{"name":"Spring 2026","application_start_date":"2026-09-01T00:00:00Z","application_end_date":"2026-09-30T00:00:00Z"}'
# Expected: 201, "status":"open"
```

### 1c. Submit the program

```bash
curl -s -X PATCH $BASE/programs/$PROGRAM_ID $AUTH \
  -d '{"status":"submitted"}'
# Expected: 200, "status":"submitted"
```

### 1d. Submit without a term — expect 422

```bash
# Create a fresh program with no terms, then try to submit
curl -s -X PATCH $BASE/programs/<new-program-id> $AUTH \
  -d '{"status":"submitted"}'
# Expected: 422, error message about needing at least one open term
```

### 1e. Approve (reviewer action)

```bash
curl -s -X PATCH $BASE/programs/$PROGRAM_ID $AUTH \
  -d '{"status":"published"}'
# Expected: 200, "status":"published"
```

### 1f. Hide — blocked by pending application (tested in Scenario 3c)

### 1g. Archive

```bash
curl -s -X PATCH $BASE/programs/$PROGRAM_ID $AUTH \
  -d '{"status":"archived"}'
# Expected: 200, "status":"archived"
```

---

## Scenario 2 — Open-Term Cap

```bash
# Create 4 open terms on a program — all succeed
for i in 1 2 3 4; do
  curl -s -X POST $BASE/programs/$PROGRAM_ID/terms $AUTH \
    -d "{\"name\":\"Term $i\"}" | jq .status
done
# Expected: all "open"

# Attempt a 5th open term — expect 422
curl -s -X POST $BASE/programs/$PROGRAM_ID/terms $AUTH \
  -d '{"name":"Term 5"}'
# Expected: 422, error about 4-term cap
```

---

## Scenario 3 — Mentor Invitation Flow

### 3a. Invite a mentor

```bash
curl -s -X POST $BASE/programs/$PROGRAM_ID/members $AUTH \
  -d '{"user_id":"<mentor-user-id>","member_type":"mentor","status":"invited"}'
# Expected: 201, "status":"invited"
MEMBER_ID=<id from response>
```

### 3b. Mentor accepts via token link

```bash
# In real flow the token is in the invite email; here we simulate with a test token
curl -s -X POST $BASE/mentor-invites/<jwt-token>/accept
# Expected: 200, "status":"active"

# Verify
curl -s $BASE/programs/$PROGRAM_ID/members | jq '.items[] | select(.id=="'$MEMBER_ID'") | .status'
# Expected: "active"
```

### 3c. Self-request flow

```bash
curl -s -X POST $BASE/programs/$PROGRAM_ID/members $AUTH \
  -d '{"user_id":"<another-mentor-id>","member_type":"mentor","status":"requested"}'
# Expected: 201, "status":"requested"

# Program Admin approves
curl -s -X PATCH $BASE/programs/$PROGRAM_ID/members/<new-member-id> $AUTH \
  -d '{"status":"active"}'
# Expected: 200, "status":"active"
```

---

## Scenario 4 — Mentee Application & Task Flow

### 4a. Apply to an open term within window

```bash
TERM_ID=<id from 1b>
curl -s -X POST $BASE/program-terms/$TERM_ID/applications $AUTH \
  -d '{"user_id":"<mentee-user-id>","role":"mentee"}'
# Expected: 201, "status":"pending"
APP_ID=<id from response>

# Verify prerequisite tasks were cloned
curl -s $BASE/applications/$APP_ID/tasks | jq '[.items[] | .category]'
# Expected: array of "prerequisite"
```

### 4b. Apply outside window — expect 422

```bash
# Create a term whose application window has already closed, then try to apply
curl -s -X POST $BASE/program-terms/<closed-window-term-id>/applications $AUTH \
  -d '{"user_id":"<mentee-user-id>","role":"mentee"}'
# Expected: 422, error about application window
```

### 4c. Mentee advances tasks

```bash
TASK_ID=<prerequisite task id>
# Mentee: incomplete → in_progress
curl -s -X PATCH $BASE/tasks/$TASK_ID $AUTH -d '{"status":"in_progress"}'
# Expected: 200

# Mentee: in_progress → submitted
curl -s -X PATCH $BASE/tasks/$TASK_ID $AUTH -d '{"status":"submitted"}'
# Expected: 200

# Verify tasks_submitted flag when last task submitted
curl -s $BASE/applications/$APP_ID | jq .tasks_submitted
# Expected: true (when all prerequisite tasks are submitted/complete)
```

### 4d. Admin tries to set complete — succeeds; mentee tries to set complete — fails

```bash
# Admin (program_admin): submitted → complete — OK
curl -s -X PATCH $BASE/tasks/$TASK_ID $AUTH -d '{"status":"complete"}'
# Expected: 200

# Mentee caller tries to set complete directly — expect 403
curl -s -X PATCH $BASE/tasks/$TASK_ID $MENTEE_AUTH -d '{"status":"complete"}'
# Expected: 403
```

### 4e. Accept application (requires attendance_type)

```bash
# Without attendance_type — expect 422
curl -s -X PATCH $BASE/applications/$APP_ID $AUTH -d '{"status":"accepted"}'
# Expected: 422

# With attendance_type — succeeds
curl -s -X PATCH $BASE/applications/$APP_ID $AUTH \
  -d '{"status":"accepted","attendance_type":"full_time"}'
# Expected: 200, "status":"accepted", "attendance_type":"full_time"
```

### 4f. Graduate a mentee

```bash
curl -s -X PATCH $BASE/applications/$APP_ID $AUTH -d '{"status":"active"}'
# Expected: 200

curl -s -X PATCH $BASE/applications/$APP_ID $AUTH -d '{"status":"graduated"}'
# Expected: 200
```

---

## Scenario 5 — Hide Guard

```bash
# With active/pending application, attempt to hide — expect 409
curl -s -X PATCH $BASE/programs/$PROGRAM_ID $AUTH -d '{"status":"hidden"}'
# Expected: 409, error listing blocking application count

# After graduating / declining all blocking applications, hide succeeds
curl -s -X PATCH $BASE/programs/$PROGRAM_ID $AUTH -d '{"status":"hidden"}'
# Expected: 200, "status":"hidden"

# Unhide
curl -s -X PATCH $BASE/programs/$PROGRAM_ID $AUTH -d '{"status":"published"}'
# Expected: 200, "status":"published"
```

---

## Scenario 6 — Term Close Guard

```bash
# With accepted application, close attempt — expect 409
curl -s -X PATCH $BASE/program-terms/$TERM_ID $AUTH -d '{"status":"closed"}'
# Expected: 409

# After all accepted apps are graduated/declined, close succeeds
curl -s -X PATCH $BASE/program-terms/$TERM_ID $AUTH -d '{"status":"closed"}'
# Expected: 200
```

---

## Scenario 7 — Bulk Decline

```bash
curl -s -X POST $BASE/program-terms/$TERM_ID/applications/bulk-decline $AUTH
# Expected: 200, { "declined_count": N }

# Verify no pending applications remain
curl -s "$BASE/program-terms/$TERM_ID/applications?status=pending" | jq .meta.total
# Expected: 0
```

---

## Scenario 8 — CSV Export

```bash
curl -s "$BASE/program-terms/$TERM_ID/applications/export?status=accepted" $AUTH \
  -H 'Accept: text/csv'
# Expected: 200, Content-Type: text/csv, rows with id,user_id,role,status,...
```

---

## Scenario 9 — Discovery Labels

```bash
curl -s $BASE/program-terms/$TERM_ID | jq .discovery_label
# Expected: "Apply Now" | "Coming Soon" | "In Progress" | "Completed"
# depending on current date vs. application window
```

---

## Success Criteria Verification

| SC | How to verify |
|----|--------------|
| SC-001 | Run Scenario 1 end-to-end; assert no unexpected status values in DB |
| SC-002 | Run Scenario 6; assert 409 returned with accepted apps present |
| SC-003 | Run Scenario 5; assert 409 returned with blocking apps present |
| SC-004 | Run Scenario 4e without `attendance_type`; assert 422 |
| SC-005 | Submit all tasks in Scenario 4c; assert `application.status` remains `pending` |
| SC-006 | Attempt reapply from `declined` (Scenario 4b variant); assert 422 |
| SC-007 | Run Scenario 2; assert 5th term creation returns 422 |
| SC-008 | Run Scenario 9 at different simulated times; assert label matches spec table |
