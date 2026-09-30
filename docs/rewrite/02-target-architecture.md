<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 02: Target Architecture

Status: Proposal — for Architecture team review
Related: [01-current-system.md](./01-current-system.md), [03-migration-plan.md](./03-migration-plan.md), [04-authorization-model.md](./04-authorization-model.md)

## Summary

Rewrite LFX Mentorship following the pattern proven by the Crowdfunding rewrite ([lfx-crowdfunding](https://github.com/linuxfoundation/lfx-crowdfunding)):

- **This repo (`lfx-mentorship`)** — public monorepo, Go backend + Nuxt frontend, same layout as `lfx-crowdfunding`.
- **PostgreSQL** (own `mentorship` schema on the shared LFX v2 RDS) replaces DynamoDB + Elasticsearch.
- **Kubernetes** (LFX v2 cluster, Helm charts, ArgoCD GitOps) replaces Lambda + Serverless Framework.
- **Behind the v2 API Gateway**, with Heimdall + OpenFGA authorizing at the edge — the idiomatic v2 pattern, since Mentorship programs are always subordinated under LF projects. See [04](./04-authorization-model.md).
- **Nuxt 4 SSR BFF** replaces the Angular 15 SPA for the public site; management moves to **LFX Self Serve**.
- **Feature parity** with today's user-facing behavior (with the explicit exclusions listed below). Milestone 1 epic: [linuxfoundation/lfx-self-serve#1477](https://github.com/linuxfoundation/lfx-self-serve/issues/1477) (Mentee public site).

## System context

```mermaid
flowchart TB
    ADMIN(["Program Admin"])
    USERS(["Mentee / Mentor"])
    SUPER(["LF Staff<br/>(approver team)"])

    subgraph GW["LFX v2 API Gateway"]
        HEIMDALL["Heimdall<br/>authN + authZ at the edge"]
        FGA[("OpenFGA")]
    end

    subgraph K8S["NEW — lfx-mentorship on LFX v2 Kubernetes"]
        NUXT["Nuxt 4 Server (BFF)<br/>public discovery · apply · initial program creation"]
        API["Go API (Chi)<br/>REST /v1"]
        PG[("PostgreSQL<br/>mentorship schema<br/>shared LFX v2 RDS")]
        CRONS["CronJobs:<br/>term-status · cf-funding-sync · task-submission-status"]
    end

    SS["LFX Self Serve<br/>manage programs, applications, tasks"]
    SYNC["fga-sync"]
    AUTH0["Auth0"]
    S3PUB[("S3 public bucket<br/>logos")]
    S3PRIV[("S3 private bucket<br/>resumes, submissions")]
    CDN["CloudFront<br/>(OAC → public bucket)"]
    EMAIL["lfx-v2-email-service<br/>NATS → SES"]
    CFAPI["Crowdfunding API"]
    SF[("Snowflake")]
    DASH["Dashboards"]

    ADMIN & USERS --> NUXT
    ADMIN & USERS & SUPER --> SS
    NUXT --> HEIMDALL
    SS --> HEIMDALL
    HEIMDALL <--> FGA
    HEIMDALL -- "authorized requests<br/>+ principal claim" --> API
    NUXT --> AUTH0
    API --> PG
    API --> S3PUB
    API --> S3PRIV
    S3PUB --> CDN
    USERS --> CDN
    API -- "NATS send_email" --> EMAIL
    API -- "outbox → NATS" --> SYNC
    SYNC --> FGA
    CRONS --> PG
    CRONS -- "M2M: funding stats" --> CFAPI
    PG -- "Fivetran (Postgres connector)" --> SF
    SF --> DASH
    SF --> CFAPI
```

Every request carrying the UID of a modelled object is authorized by Heimdall against OpenFGA before it reaches the API; the service itself makes no authorization decisions. The qualifier is load-bearing: a few current routes carry an ID whose type is not in the model (program terms, self-service user writes) and so have nothing to check — decision 7 in [04](./04-authorization-model.md) reshapes them rather than leaving them authentication-only. Tuples flow the other way — the API emits them via a transactional outbox to fga-sync. See [04](./04-authorization-model.md) for the model and the emission contract.

## Repository layout

```
lfx-mentorship/
├── backend/
│   ├── cmd/                # mentorship-api + cron binaries
│   ├── internal/
│   │   ├── domain/         # models, repository interfaces
│   │   ├── service/        # business logic
│   │   ├── handler/        # Chi routes, request/response
│   │   └── infrastructure/ # postgres repos, auth0 middleware, clients (crowdfunding, email, s3)
│   ├── db/migrations/      # golang-migrate SQL
│   └── charts/             # Helm chart
├── frontend/
│   ├── app/                # Nuxt 4: pages, components, composables
│   ├── server/             # BFF: auth routes, middleware
│   └── charts/             # Helm chart
└── docs/
```

Same layered architecture, DI-by-constructor, and repository-interface pattern as the Crowdfunding backend.

## Data model (proposal-level ERD)

Relational schema replaces 8 DynamoDB tables + 30 GSIs. Program terms become first-class rows instead of documents nested in projects.

```mermaid
erDiagram
    users ||--o{ user_profiles : has
    users ||--o{ applications : submits
    users ||--o{ program_members : "participates as"
    programs ||--o{ program_terms : has
    programs ||--o{ program_members : has
    programs ||--o{ program_skills : requires
    programs ||--|| program_funding_stats : "caches CF stats"
    program_terms ||--o{ applications : receives
    applications ||--o{ tasks : "works on"
    programs ||--o{ invitation_tokens : issues

    users {
        uuid id PK
        text username "LFID"
        text email
    }
    programs {
        uuid id PK
        text name
        text slug
        text status "draft | submitted | published | rejected | archived | hidden"
        text project_uid "LF project this program belongs to"
        uuid cf_initiative_id "link to Crowdfunding initiative"
    }
    program_terms {
        uuid id PK
        uuid program_id FK
        daterange term_dates
        daterange application_window
        text status
    }
    applications {
        uuid id PK
        uuid program_term_id FK
        uuid user_id FK
        text role "mentor | mentee"
        text status "pending | accepted | declined | withdrawn | graduated"
    }
    program_members {
        uuid id PK
        uuid program_id FK
        uuid user_id FK
        text member_type "program_admin | mentor"
        text status
    }
    tasks {
        uuid id PK
        uuid application_id FK "NOT NULL — parent for permission inheritance"
        text category "prerequisite | non_prerequisite"
        text status "incomplete | in_progress | complete | submitted"
        date due_date
    }
```

Notes:

- **Search**: PostgreSQL full-text search (`tsvector` + GIN indexes) over programs, skills, and profiles replaces the Elasticsearch cluster and its 8 sync jobs. Data volume (thousands of rows) is far below where a dedicated search engine pays for itself.
- **Denormalization jobs eliminated**: mentor lists, skill mappings, and counts become queries/views instead of cron-materialized copies.
- **Funding stats**: `program_funding_stats` is an hourly-refreshed local cache of Crowdfunding data (see Integrations) — the same pattern Crowdfunding uses for Ledger stats.
- **No enrollment entity, and no mentor assignment.** The application *is* the lifecycle object — one row per user per term, whose status runs `pending → accepted → graduated` (there is no `active` application status; AQ-10 in [04](./04-authorization-model.md) resolved it as dropped). This matches legacy, where acceptance and graduation are status changes on a single `program-term-mentees` row — the term-keyed mentee source ([00](./00-current-authz-relations.md)); `project-members` is the program-level membership table and carries no term identity — and mentors relate to the **program**, not to individual mentees (the legacy per-mentee "mentors" list is a cron-denormalized copy of the program's approved mentors). Tasks therefore hang off the application, with `category` distinguishing `prerequisite` from `non_prerequisite` tasks. Introducing `enrollments` + `enrollment_mentors` would add a parity feature nobody asked for; see "No enrollment entity" and decision 2 in [04](./04-authorization-model.md).
- **`hold` is not a valid application status.** The merged schema's `applications_status_check` still permits it today (`backend/db/migrations/001_initial.up.sql:184`), but it describes a *paused accepted mentorship*, not an application outcome, and does not belong in this enum — a follow-up migration drops it from the constraint. The ERD above omits it accordingly.
- Exact column-level schema is an implementation-phase deliverable; this ERD fixes the entity boundaries.

## Frontend split: Nuxt public site + Self Serve management

Same split as Crowdfunding (public site + `app.lfx.dev` lenses):

| Surface                                                                                  | Audience                                            | Scope                                                                                                                                                                                                                                                                                                                                                     |
| ---------------------------------------------------------------------------------------- | --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Nuxt 4 public site** (new, in this repo)                                               | Unauthenticated visitors, applicants                | Marketing/overview, program discovery & search, program detail, apply flow, initial program creation. SSR for SEO. LFX Insights design language (per crowdfunding.linuxfoundation.org). Milestone 1 epic: [lfx-self-serve#1477](https://github.com/linuxfoundation/lfx-self-serve/issues/1477) (Mentee public site — tracked in the lfx-self-serve repo). |
| **LFX Self Serve** ([lfx-self-serve](https://github.com/linuxfoundation/lfx-self-serve)) | Authenticated program admins, mentors, mentees, admins | Manage programs, review applications, tasks/milestones, admin approvals. Delivered as separate Admin/Mentor/Mentee management epics tracked in [lfx-self-serve](https://github.com/linuxfoundation/lfx-self-serve).                                                                                                                                       |

Frontend stack mirrors Crowdfunding: Nuxt 4 + Vue 3, TypeScript, Tailwind + PrimeVue, Pinia + Vue Query, Vitest + Playwright.

## Authentication and authorization

**Authorization happens at the edge, not in the service.** Mentorship programs are always subordinated under LF projects, so this service adopts the idiomatic v2 pattern — behind the API Gateway, with Heimdall authorizing each route against OpenFGA — rather than Crowdfunding's interim standalone-API model. [04-authorization-model.md](./04-authorization-model.md) is the detailed proposal; the summary:

### Authentication

- **Users**: OAuth2 PKCE via Auth0; tokens in HTTP-only session cookies (never exposed to JS).
- **Identity**: the **`principal`** claim on the Heimdall-issued JWT, populated by the platform's `create_jwt` finalizer. The upstream Auth0 `sub` is deliberately not forwarded to services, so `principal` is both the caller's identity and the key for `user:{lfid}` tuples.
- **M2M**: client-credentials for the CF funding-stats sync. The credential is **outbound** — the CronJob calls Crowdfunding's API with it (see Integrations). Mentorship serves no inbound `/v1/internal/*` route today, and should not grow one: an internal API on the shared gateway host would need its own authorization story (see [05](./05-heimdall-gateway.md)).
- **Self Serve**: silent secondary auth for the shared gateway audience (`https://lfx-api.{lfx.domain}/`), same mechanism it already uses for Crowdfunding. Behind Heimdall there is no per-service Auth0 audience to acquire — the gateway audience covers every service on the shared host, and the service-specific audience appears only on the Heimdall-issued token (`lfx-mentorship-backend`), which the caller never requests. See [05](./05-heimdall-gateway.md).

### Authorization

- **Every route carrying the ID of a modelled object gets a Heimdall RuleSet** checking a single FGA relation. The service performs no ownership or role checks. The qualifier is the same one as above: routes whose ID names an unmodelled type (program terms, self-service user and profile writes) have no relation to check, so decision 7 in [04](./04-authorization-model.md) reshapes them — nesting them under a modelled parent or moving them to `/me` — rather than leaving them authentication-only.
- **Relations live in OpenFGA, derived from Postgres.** Postgres remains the system of record for membership; the API emits tuples through a transactional outbox to fga-sync at each state transition.
- **`programs.project_uid` is what makes the project link derivable.** Inherited permissions depend on a `mentorship_program#project@project:{uid}` tuple, so the owning LF project must be a persisted column — the outbox re-derives payloads from current Postgres state and cannot invent it. Legacy already carries this as `lfProjectId`, and `CreateProject` requires it (`project/service.go:195`), but programs created before that rule predate it — legacy has a dedicated `GetProgramsWithLFProjectID` query precisely because the field is not universally populated. The backfill must therefore report unmapped programs rather than silently importing them: a program with no `project_uid` has no parent to inherit from and would be authorized only by its direct grants. Making the column `NOT NULL` is the forcing function; resolving the stragglers is a Backfill-phase task ([03](./03-migration-plan.md)).
- **`tasks.application_id` must become `NOT NULL` for the same reason.** Task permissions derive from the parent application (decision 2 in [04](./04-authorization-model.md)), so a task with no parent has nothing to inherit from. The merged schema makes the column nullable — `application_id UUID REFERENCES applications(id) ON DELETE SET NULL` (`backend/db/migrations/001_initial.up.sql:199`) — and the migration script deliberately imports legacy tasks it cannot match to an application with a NULL parent rather than dropping them (`backend/db/scripts/migrate_dynamo_to_postgres.py:899-901`, counted as `unresolved`). Those rows emit no `mentorship_task#mentorship_application@mentorship_application:{id}` tuple, so `manager` — which resolves only as `reviewer from mentorship_application` — finds nothing and every review check on the task fails closed. Same three parts as `project_uid`, in the same order: an **unmapped-task report** from the Backfill phase, resolution of the stragglers, then `NOT NULL` as the forcing function ([03](./03-migration-plan.md)). `ON DELETE SET NULL` also has to go — orphaning a task on application deletion reintroduces the same hole at runtime, so the constraint becomes `ON DELETE CASCADE`.
- **The residue is `/me/*`, and it is self-scoping rather than authorization.** For **list** endpoints the service filters rows by the caller's `principal` — data scoping on the caller's own records, not a grant/deny decision. GW-5 in [05](./05-heimdall-gateway.md) extends the same shape to **self-service writes** on the caller's own user and profile, which become `/me` routes rather than the ID-addressed `PATCH/DELETE /v1/users/{id}` they are today: the target is derived from `principal`, never from request input or a path ID. The invariant is therefore that `/me/*` **never addresses another subject's object** — it is not a second way to reach an arbitrary object by ID, which is what would need an edge check.

Three things this replaces from the Crowdfunding-derived design:

| Was | Now |
| --- | --- |
| Service-layer role checks against `program_members` / `enrollments` | Heimdall + FGA relations at the edge |
| Super-admin LFID allowlist injected at deploy time | `member` on `mentorship_approver_team:global`, a dedicated approver-team type this service owns (AQ-5 in [04](./04-authorization-model.md), resolved; the type and how approvers read a non-public program are settled — AQ-8's type, AQ-9 — but AQ-8's roster provisioning and administration authority remain an open blocker) |
| HMAC-signed email approval links, no login | Authenticated approval in Self Serve, gated on approver-team membership |

The allowlist and the HMAC links were each a second authorization mechanism outside the model; both are retired.

## Integrations

| Service        | Direction             | Mechanism                                                                                                                                                                                                                                                                    |
| -------------- | --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Crowdfunding   | Mentorship → CF       | **CronJob calls CF API (M2M `access:manage`), caches funding stats (`amountRaised`, etc.) in `program_funding_stats`.** Replaces legacy SNS/SQS eventing and the Snowflake round-trip for serving-path data. If CF is unavailable, Mentorship serves the last cached values. The funding-stats endpoint is a **new Crowdfunding-repo deliverable** (no such M2M route exists in CF today): exposed under `access:manage`, keyed by `cf_initiative_id`, contract defined with the CF team during Build. |
| Snowflake      | Mentorship → SF       | Fivetran **Postgres** connector (replacing the DynamoDB connector); existing `fivetran_mentorship_*` dbt models repointed. Feeds dashboards and CF analytics. Analytics-plane only — never in the serving path.                                                              |
| Auth0          | both                  | PKCE (users), client-credentials (M2M). After the gateway cutover Auth0 sits **in front of Heimdall** and its token never reaches this service — the API middleware validates the **Heimdall**-issued JWT against the cluster-internal Heimdall JWKS, not Auth0's. See [05](./05-heimdall-gateway.md) for the two token contracts and the dual-accept window. |
| Email          | Mentorship → NATS     | All transactional email (invitations, application status, task notifications, program-submission notification to LF staff — a notification only, no longer a signed approval link) goes through **[lfx-v2-email-service](https://github.com/linuxfoundation/lfx-v2-email-service)** — NATS request/reply on `lfx.email-service.send_email` over Amazon SES, via its Go client `pkg/api`. No service sends its own email, so Mentorship holds no SMTP or provider credentials. The relay is **pre-rendered only** (no templating), so this repo owns the templates and the Go backend renders `html` and `text` per notification — replacing the legacy Mandrill arrangement, where ~47 templates lived in Mailchimp's editor and were hand-synced. Mandrill is legacy-only and out of scope; the rail decision is recorded in [07-email-delivery.md](./07-email-delivery.md) ([linuxfoundation/lfx-self-serve#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)). Known limits to design around: **no send retry** in the relay, no attachments, no CC/BCC, and a non-prod recipient-domain allowlist. `Notifier` stays fire-and-forget — a failed send must never fail the business operation. |
| S3             | Mentorship → S3       | Program logos and profile logos (public); resumes and task submission files (private). **Uploads go through the API, not presigned URLs** — see [Object storage](#object-storage) below. This supersedes the earlier "presigned URLs, as in CF" note: CF's pattern stores every object in one world-readable bucket, which is wrong for the resumes and submissions Mentorship holds. |
| LFX Self Serve | SS → Mentorship       | User tokens through the API Gateway; Heimdall authorizes each route against FGA before it reaches the service                                                                                                                                                                |

## Object storage

Mentorship follows the platform object-store design ([lfx-object-store-design](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-design/SKILL.md), provisioned per [lfx-object-store-ops](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-ops/SKILL.md)) rather than Crowdfunding's presigned-PUT pattern. Two rules drive everything below: **uploads go through the API** (browsers never write to S3 directly), and **public and private files live in separate buckets** — a CDN's origin authorization can read every key in its origin bucket, so one mixed bucket would make resumes anonymously retrievable to anyone who knows the key.

Crowdfunding's objects are initiative logos, public either way, so a single world-readable bucket costs it nothing. Mentorship stores resumes and task submissions, and that difference is the whole reason the pattern is not copied.

**Implementers: read both skills before writing code or HCL.** This section states what Mentorship stores and where; it does not restate the platform baseline. [lfx-object-store-design](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-design/SKILL.md) owns the application side — SDK wiring and the credential chain, `EnsureBucket`, the chart contract (`s3.endpointURL`, `s3.createMissingBucket`, `cdnURLPrefix`, the nats-s3 sidecar) — noting that Mentorship needs two buckets, so those values repeat per bucket under the skill's namespacing convention (`LOGOS_S3_BUCKET`/`ATTACHMENTS_S3_BUCKET`, `LOGOS_CDN_URL_PREFIX` on the public bucket only — **`CDN_URL_PREFIX` below is shorthand for `LOGOS_CDN_URL_PREFIX`**, the only public prefix Mentorship has), while process-wide settings such as `AWS_REGION` and the credential chain are not namespaced, and the local stack. [lfx-object-store-ops](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-ops/SKILL.md) owns provisioning, in [lfx-v2-opentofu](https://github.com/linuxfoundation/lfx-v2-opentofu) rather than this repo. Where this document and a skill disagree, the skill is the baseline and the difference is a bug in this document — raise it rather than diverging silently. **One divergence is deliberate and named here.** The skill's cache-busting model is a stable key overwritten in place plus a `?v=` query parameter; Mentorship instead **writes a fresh key for every upload and never overwrites** ([§stored value and delivery](#stored-value-and-delivery)). That is the scheme the legacy platform already uses, which is what lets the migration keep every object key unchanged ([03 §S3 objects](./03-migration-plan.md#migration-specific-tasks)), and it removes the `?v=` token, the CDN cache-key configuration and the overwrite serialization the skill's model needs. It is raised upstream against the skill rather than adopted silently. Persisting the composed CDN URL in the public columns rather than the bare key is not a divergence — the skill defines `public_url` as `CDN_URL_PREFIX` plus key, and [lfx-v2-member-service](https://github.com/linuxfoundation/lfx-v2-member-service) already ships it persisted.

### File classes

| Class | Columns | Bucket | Served by |
| --- | --- | --- | --- |
| Program logos | `programs.logo_url` | public | CDN (`CDN_URL_PREFIX`) |
| Profile logos | `user_profiles.logo_url` | public | CDN |
| Resumes | `user_profiles.resume_file` (lifted out of `profile_links.resumeLink`) | **private** | service download route |
| Task submissions | `tasks.file` | **private** | service download route |

**Avatars are not a file class.** The legacy platform has no avatar upload and neither does this one: `users.avatar_url` is written by the service — legacy takes the OIDC `picture` claim at login (`jobspring/backend/auth/handler.go:104`) and overwrites it with the profile logo on profile save (`jobspring/backend/user/service.go:376`) — so it holds either a foreign URL or a profile-logo URL and is rendered from wherever it points. The column is unchanged and no file route writes it. The one schema change — `resume_file` — is the work in [linuxfoundation/lfx-self-serve#1539](https://github.com/linuxfoundation/lfx-self-serve/issues/1539), and lands before the ETL runs ([03 §migration-specific tasks](./03-migration-plan.md#migration-specific-tasks)).

`tasks.submit_file` is not a file reference — it is the template flag (`required` / unset) saying whether a task demands an upload. The schema comment on `001_initial.up.sql:219` suggesting it may hold a URL is wrong, as is the matching row in `specs/001-mentorship-core-workflow/data-model.md:196`. Both corrections are folded into the schema work tracked in [linuxfoundation/lfx-self-serve#1539](https://github.com/linuxfoundation/lfx-self-serve/issues/1539), which edits that migration anyway.

### Stored value and delivery

**Every upload writes a new object under a fresh key and never overwrites an existing one.** The key is `{uuid}-{filename}` at the bucket root — a random UUID, a hyphen, and the client's filename sanitised to `[A-Za-z0-9._-]` and capped in length, extension kept — with the sniffed `Content-Type` set on the object. This is the legacy scheme (`jobspring/backend/upload/service.go:47-49`), kept so that migrated and new objects are one population: the bucket says which class an object belongs to, the key says nothing about its owner, and the filename travels with the object. Because a key is never rewritten, a stored URL is either current or dangling, never stale — so there is no cache-busting parameter, no CDN query-string configuration, no `ttl` override, and no reason for two writers to coordinate on one key.

What the column stores differs by class:

- **Public files** store the **full CDN URL**, `{CDN_URL_PREFIX}/{key}` with the key percent-encoded, written once at upload and returned verbatim as `public_url`. `Cache-Control: public, max-age=86400` is set on the object at upload — the platform baseline, kept even though the bytes behind a key never change, because the skill rules out `immutable` and one convention is enough. Two consequences. `CDN_URL_PREFIX` is baked into a data column, so changing the CDN hostname is a prefix-substitution pass over the public-class rows. And the prefix is **required configuration** in every deployed environment — `templates/validate.yaml` refuses to render without it — with local development pointing it at the skill's `nginx-s3-gateway` stand-in; the public-class service `GET` routes exist because the skill requires them ([06 §file routes](./06-route-matrix.md#file-routes)), but no stored value ever names them. A public column may also hold a **foreign URL** — an LF project logo, an Auth0 picture — carried through by the migration; it is returned as stored, and no object in the bucket stands behind it.
- **Private files** store the **object key**, not a URL and not a route. The download route is derived from the owning entity (`GET /v1/tasks/{uid}/submission/file`, `GET /v1/applications/{uid}/resume`), streams `GetObject`, passes through its `Content-Type` and `Content-Length`, and sets `Content-Disposition: attachment` with the filename taken from the key after the first hyphen — falling back to a generated name such as `resume.pdf` when a legacy key carries only an extension, which the legacy frontend has produced for every upload since it began stripping names (`lfx-mentorship-upgrade/src/app/core/file-upload.service.ts:38`). No companion metadata columns: the name comes from the key, the type and size from the object. The bucket has no CDN attached.

  **The key is repository-internal and must never appear in a response body.** `tasks.file` and `profile_links.resumeLink` are serialized straight to the client today (`Task.File`, `UserProfile.ProfileLinks`). Every response that carries them — including the program-admin mentee listing, which returns a per-task uploads list — returns the **download route** instead, and omits it for callers the ruleset does not admit. Until the `resumeLink` backfill has drained, the projection also drops `resumeLink` alone from `profile_links` while returning `linkedinProfileLink` and `githubProfileLink`.

**Replace is three steps, in order:** `PutObject` the new key; update the row with a conditional `WHERE` on the previous value; `DeleteObject` the previous key. If the row update matches nothing — a concurrent replace won — delete the key just written instead. If `PutObject` succeeds and the commit fails, delete the new key: the row still points at the old object, which was never touched, so there is no rollback and no `VersionId` handling.

Per-file cap is **20 MB** — the platform ceiling, not the per-class limit. **Image classes (program logos, profile logos) cap at 2 MB**, matching every shipped V2 image path: member-service's org logo (`pkg/constants/logo.go:13`) and `lfx-self-serve`'s avatar, which shipped at 20 MB and was rolled back to 2 MB (LFXV2-2628). A 20 MB PNG behind `max-age=86400` on an asset rendered in every list view is the cost that rollback was fixing. Server-side downscaling is **out of scope** for the rewrite — member-service downscales rasters above a dimension bound, and Mentorship may adopt that later, but the 2 MB cap is a reject, not a resize. No presigned uploads and no resumable/chunked uploads. Buckets are private with versioning, SSE, and lifecycle rules. Write access in **deployed** environments is via IRSA; the local `nats-s3` sidecar uses a locally generated static SigV4 pair, which is the platform's documented local mode and must never appear in a deployed values file.

### Deleting the bytes

**Every path that drops a file's locator must also remove the object. Bucket lifecycle is not a substitute.** The ops lifecycle rules expire *noncurrent* versions ([lfx-object-store-ops](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-ops/SKILL.md)); a current object whose last database reference is gone is unreferenced, not noncurrent, and nothing reaps it — for a resume or submission, live PII with no expiry. The paths that need it, only the second covered by the file `DELETE` routes in [06](./06-route-matrix.md):

- **Replacement.** The previous key, as in [§stored value and delivery](#stored-value-and-delivery).
- **Parent deletion.** `DELETE /v1/programs/{uid}`, `DELETE /v1/tasks/{uid}`, `DELETE /v1/me`, and `DELETE /v1/me/profile` all remove rows that hold file locators — as do application and term deletes, since `tasks.application_id` is `ON DELETE CASCADE` (`002_authorization_foundation.up.sql:82-83`) and removes task rows and their locators without touching those routes. `user_profiles.user_id` is `ON DELETE CASCADE` too, so deleting a user erases `resume_file` in Postgres without touching S3. Each must enumerate the file keys the deleted rows held and remove them.
- **A write that half-succeeds.** The key just written, as above.

**Removal is a plain `DeleteObject`.** On the versioned buckets that leaves a delete marker and a noncurrent version, which the lifecycle rule expires after 30 days (`noncurrent_days: 30` in `object-store-definitions.yaml`); in that window only a principal holding `s3:GetObjectVersion` can read it, which neither the service role nor the CDN origin access has. **That lag is accepted** in exchange for not widening the IAM baseline — no `s3:DeleteObjectVersion`, no `s3:ListBucketVersions`, no HCL change in lfx-v2-opentofu. Deletion may lag the row delete, but must be retried to completion rather than dropped — a failure leaves PII behind with nothing pointing at it. **There is no scheduled sweep to fall back on:** a recurring unreferenced-object sweep is not in scope here, so each path above must clean up after itself.

### Routes

Upload and download are ordinary API routes, authorized by Heimdall like any other — the full rows are in [06](./06-route-matrix.md). Private downloads stream from S3 after the ruleset authorizes the request, with `Content-Disposition: attachment` and `Range` pass-through, and **`Cache-Control: private, no-store`** on both the response and the stored object metadata — resumes and submissions are PII, and the default would otherwise let a browser or an intermediary retain an authenticated response. The migration copy sets it explicitly for the same reason it sets `ContentType`: legacy objects predate the split and may carry a public policy.

**The CDN is not an authorized path.** It serves the object to anyone holding the URL, with no relation check at all, which is why only classes that are public by intent may be CDN-fronted (RM-6). The public-class service `GET` routes carry the same relation as the record that holds the URL where the record has one — `GET /v1/programs/{uid}/logo` (`viewer`) — and are `allow_all` with a service-side publicly-listable check for profiles, since there is no profile object type in the model ([06 §file routes](./06-route-matrix.md#file-routes)).

Traefik needs `maxRequestBodyBytes` above `20971520`, with margin for multipart overhead, on the upload routes. Attach that `Buffering` middleware to the **upload routers only** — the platform skill phrases the download side as `responseBuffering: false`, but Traefik's `Buffering` CRD has no such field, so the way to keep downloads streaming is simply not to attach the middleware to them. Buffering a 20 MB response would also defeat the `Range` pass-through the private download routes rely on.

## Kubernetes resources

- **Deployments**: `mentorship-api` (Go), `mentorship-frontend` (Nuxt) — each with Service + Ingress, Helm charts in-repo, deployed via ArgoCD ([lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd)).
- **CronJobs** (3, down from 15+ Lambda jobs):
  1. `term-status` — open/close program terms and application windows by date.
  2. `cf-funding-sync` — hourly funding-stats cache refresh from the Crowdfunding API.
  3. `task-submission-status` — task submission status rollups.
- **Database**: shared LFX v2 RDS, `mentorship` schema, credentials via K8s secrets ([lfx-secrets-management](https://github.com/linuxfoundation/lfx-secrets-management)).
- CI/CD mirrors Crowdfunding: GitHub Actions (test, lint, image build → GHCR), MegaLinter, Trivy.

## Explicitly out of scope (initial release)

| Dropped                                | Rationale                                                                                                                                                                                              |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Employer portal**                    | Not reachable from the production UI (no entry point on `/participate`); vestigial. Pre-decommission data check in the migration plan. Related marketing copy ("referred to employers") to be removed. |
| **Elasticsearch**                      | Replaced by Postgres FTS.                                                                                                                                                                              |
| **A Mentorship-owned email sender**    | No direct Mandrill, SES, or SendGrid client in this repo. Sending is delegated to `lfx-v2-email-service` (see Integrations), which reaches SES on our behalf; the ~47 legacy Mandrill templates are a parity *checklist*, not a porting target.                                                                              |
| **Slack ops alerts**                   | Ops signal moves to standard K8s/CI channels.                                                                                                                                                          |
| **OpenSSF badge fetch**                | Cosmetic; can return later if wanted.                                                                                                                                                                  |
| **Observability stack**                | Deferred; not part of the initial release.                                                                                                                                                             |
| **SNS/SQS eventing with Crowdfunding** | Replaced by the CF API sync + Snowflake analytics path.                                                                                                                                                |

Everything else is **feature parity**: same roles, same program/term/application/task lifecycle, same email notifications, same discovery capability.
