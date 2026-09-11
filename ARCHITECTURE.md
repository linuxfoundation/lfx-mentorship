<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->

# LFX Mentorship — Architecture

**Scope of this file.** This is not the internal design of the Go service or the Nuxt app — it is
the **cross-component contract**: how Mentorship plugs into the rest of the LFX platform, who is
allowed to do what, and which services own what.

**This file describes the target architecture**, not the current state of the build. It is the
contract to build against. Implementation status — what is built, what is half-built, and what is a
live defect — belongs in issues and PRs, not here, so that this file stays stable as the code moves.
The one exception is §6, which records contracts that do **not** exist yet, precisely so that nobody
builds against them.

> **Authorization is the largest gap between this document and the running service.** The write
> paths do not yet perform object-level authorization, and the edge described in §3 is not deployed.
> Treat §3 as the specification, not a description of today's behavior.

Companion documents, all narrower than this one:

| Document | Purpose |
|---|---|
| [`docs/rewrite/00-current-authz-relations.md`](docs/rewrite/00-current-authz-relations.md) | The legacy authorization baseline the new model is validated against |
| [`docs/rewrite/01-current-system.md`](docs/rewrite/01-current-system.md) | The legacy platform being replaced |
| [`docs/rewrite/02-target-architecture.md`](docs/rewrite/02-target-architecture.md) | Full rewrite proposal (data model, repo layout, scope exclusions) |
| [`docs/rewrite/03-migration-plan.md`](docs/rewrite/03-migration-plan.md) | Migration phases |
| [`docs/rewrite/04-authorization-model.md`](docs/rewrite/04-authorization-model.md) | **The FGA model** — types, relations, the Postgres/FGA split, and which relation each route checks |
| [`docs/rewrite/05-heimdall-gateway.md`](docs/rewrite/05-heimdall-gateway.md) | The gateway edge that consumes it, and the cutover sequence |
| [`CLAUDE.md`](CLAUDE.md) | Conventions for working in this repo |

`00`, `04` and `05` are added by [lfx-mentorship#119](https://github.com/linuxfoundation/lfx-mentorship/pull/119),
which is still under Architecture-team review — **so the links to them above and in §3 resolve only
once that PR merges, and it should merge first.** `04` and `05` remain proposals even then; §3 below
records the decisions they rest on and does not restate the model.

Where the code, the spec directory, or `docs/rewrite/` contradict the *contract* stated here, that is
a defect in one of them — say so in review rather than letting the drift stand.

---

## 1. System context

```mermaid
flowchart TB
    USERS(["Mentee / Mentor"])
    ADMIN(["Program Admin"])
    APPROVER(["Program Approver<br/>(LF staff)"])

    HEIMDALL["API Gateway (Heimdall)<br/>authorizes at the edge"]
    FGA[("OpenFGA")]

    subgraph K8S["lfx-mentorship — LFX v2 Kubernetes"]
        NUXT["Nuxt 4 SSR<br/>public discovery · apply<br/>BFF session"]
        API["Go API (Chi)<br/>REST /v1"]
        PG[("PostgreSQL<br/>mentorship schema<br/>shared LFX v2 RDS")]
        CRON["CronJob<br/>program-funding-stats-sync"]
    end

    SS["LFX Self Serve<br/>manage programs · applications · tasks"]
    AUTH0["Auth0"]
    LEDGER["Ledger API<br/>mentorship-credit transactions"]
    CF["Crowdfunding API<br/>categorized transactions · sponsors"]
    S3[("S3 uploads")]
    EMAIL["lfx-v2-email-service<br/>NATS → SES"]
    SF[("Snowflake")]

    USERS & ADMIN --> NUXT
    USERS & ADMIN & APPROVER --> SS
    NUXT <--> HEIMDALL
    NUXT --> AUTH0
    SS -- "user token" --> HEIMDALL
    HEIMDALL -- "check" --> FGA
    HEIMDALL --> API
    API --> PG
    API --> S3
    API --> EMAIL
    API -- "M2M access:manage" --> CF
    CF -- "reads" --> LEDGER
    CRON -- "API key: transactions" --> LEDGER
    CRON --> PG
    PG -- "Fivetran" --> SF
```

**Two front ends, one API.** The public Nuxt site owns unauthenticated discovery and the apply flow;
everything authenticated and management-shaped lives in LFX Self Serve. Both talk to the same `/v1`
surface, and there is no separate admin API. This split is the reason the FGA model carries a public
wildcard on programs (§3.2) — one of the two front ends does not authenticate.

---

## 2. Components owned by this repo

| Component | Runtime | Responsibility |
|---|---|---|
| `mentorship-api` | Go 1.25, Chi v5, pgx v5 | The entire REST surface (`/v1`) and all business rules |
| `mentorship-frontend` | Nuxt 4 SSR | Public site, including BFF session handling for the apply flow |
| `program-funding-stats-sync` | Go CronJob | Hourly refresh of `program_funding_stats` from the Ledger API |
| `mentorship` schema | PostgreSQL | System of record |

Both services ship as in-repo Helm charts and are deployed by ArgoCD from
[lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd)
(`values/{global,dev,staging,prod}/lfx-mentorship-{backend,frontend}.yaml`). A push to `main`
publishes `:development` images that dev tracks by digest; a `v*.*.*` tag publishes semver images and
signed OCI charts, which staging and prod pin.

---

## 3. Authorization

This is the part of the architecture the rest of the platform most depends on getting right.

### 3.1 Agreed direction

Mentorship sits **behind the LFX v2 API Gateway (Heimdall), authorizing against OpenFGA**, from the
start — rather than shipping bespoke auth and retrofitting later. Decided in architecture review on
2026-09-03 with Eric Searcy and Jordan Evans. Consequences agreed in that review:

- **Applications are their own FGA type.** Not an attribute of a program.
- **There is no `mentorship_super_admin`.** LF staff reach everything through the existing
  project `writer` relation. A new global role for the same population would be redundant, and
  `writer` is already the population that can grant roles to others.
- **Program approval is not project-scoped.** Approval is performed by a single global population
  (currently one person), so it is a **global team membership check** on the approve endpoint —
  the same pattern used for SurveyMonkey template managers — not a relation on `project`.
- **A project-level program admin relation is needed.** Someone who can manage *all* mentorship
  programs for a project, including creating new ones, without being a full project admin. This
  must surface in the Self Serve project permissions page alongside viewer/manager, not in a
  separate mentorship-only UI. Naming to be confirmed with the project-lens owners.
- **`task` is its own FGA type, parented on the application.** The 2026-09-03 call left this open —
  a task would earn a type only if it could be assigned to someone who is not already a mentor or
  mentee on the parent application. A second Architecture review settled it the other way: tasks get
  `mentorship_task`, whose parent reference is the **application**, completing one inheritance chain
  project → program → application → task. See [`04` decision 2](docs/rewrite/04-authorization-model.md).

### 3.2 The FGA model lives in `04`, not here

**[`docs/rewrite/04-authorization-model.md`](docs/rewrite/04-authorization-model.md) is the single
source of truth for the types, relations, route-to-relation mapping, and lifecycle emissions.** It is
not restated here: the model has already been revised three times across two Architecture reviews, and
a second copy maintained separately would drift from it. Four mentorship-owned types are
proposed there — `mentorship_program`, `mentorship_application`, `mentorship_task`,
`mentorship_approver_team` — plus two relations appended to the existing `project` type
(`mentorship_program_admin`, `mentorship_program_creator`). Landing them in
[`model.fga`](https://github.com/linuxfoundation/lfx-v2-helm/blob/main/charts/lfx-platform/files/model.fga)
is PR 1 of the four-PR path in `04 §implementation path`, gated on `tests.yaml` passing.

What belongs in *this* document is the handful of model properties that are cross-component contracts
rather than schema:

- **Program `viewer` carries a `[user:*]` wildcard, and it is load-bearing.** It is not "always
  public" — fga-sync writes it as a **per-object tuple** only while the program is `published`, and
  re-emits without it on the way back down to `archived`/`hidden`. It exists because Mentorship ships
  two front ends and only one of them authenticates: drop the wildcard and every request the public
  Nuxt site makes is denied at the edge. Any model variant that removes it loses one of the two UIs.
- **Approval is held deliberately outside the program's own relations.** The decision route checks
  `member` on the static object `mentorship_approver_team:global`. Folding it into `writer` would let
  every program admin approve their own program, so `PATCH /programs/{uid}` must reject a `status`
  field rather than silently ignoring it.
- **Project `writer` reaches everything by inheritance**, three levels down to tasks. There is no
  `mentorship_super_admin`, per §3.1.
- **`mentorship_program_admin` must be owned by project-service, not this service.** It sits on the
  `project` type, and project-service emits full-state `update_access` for `project` objects — so a
  tuple written independently by Mentorship is deleted by the next project update. Adding the relation
  to `model.fga` is necessary but not sufficient; it needs an owner (`04` AQ-4), and it is the one item
  a model merge alone does not make functional. Its sibling `mentorship_program_creator` is a pure
  computed union, so it needs no tuples and no owner.

### 3.3 Service-side invariants the edge cannot enforce

Heimdall authorizes the object named in the request. Three classes of check therefore stay in the
service, and they are contracts rather than implementation details:

- **Parent-child invariants on parent-authorized routes.** Wherever the edge checks a relation on a
  *parent* object while the mutation targets a *child* by its own ID, the service must verify the
  child belongs to that parent — otherwise the edge authorized a different object than the one being
  mutated. Without it, a `writer` on program A passes the edge check and then mutates a member or
  skill of program B. This is referential integrity on the request, not an access decision, which is
  why FGA holds no tuple for it (`04` decision 7). It applies to every parent-authorized route,
  including any child resource added later.
- **Canonical UIDs on authorized routes.** Tuples are keyed by UID, so a RuleSet built from a raw
  `{id}` capture that may be a slug would check a nonexistent object and deny a valid URL.
  `GET /v1/programs/resolve/{id}` returns the canonical `program.ID` for this purpose — a
  prerequisite for the RuleSets (`05` GW-2), not a cutover detail.
- **Redaction on the public read surface.** The public routes (`/programs`, `/mentees`, `/mentors`,
  summaries) are the intended discovery surface and must expose `name` only. `User` carries `email`
  and `lfid`, and `UserProfile` adds `phone`, `address`, `demographics` and `socioeconomics`; none of
  those may appear on an unauthenticated route. `05` GW-8 owns the explicit redaction contract.

### 3.4 Authentication

- Users: OAuth2 PKCE via Auth0, tokens in HTTP-only session cookies, never exposed to JS.
- LFID from the `https://sso.linuxfoundation.org/claims/username` claim (the claim name; the column
  that stores it is `users.lfid` — §5).
- Self Serve obtains a token for the Mentorship audience and forwards it — the mechanism it
  already uses for Crowdfunding.
- Mentor invite acceptance (`POST /v1/mentor-invites/{token}/accept`) is deliberately
  **unauthenticated**: the token in the path is the credential. Its entropy, single-use semantics,
  and expiry are part of the authorization contract.
- **There is a total authentication bypass for local development, and it must never reach a deployed
  environment.** `DISABLED_MOCK_LOCAL_PRINCIPAL` sets a static principal and
  `ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS=true` arms it
  ([`jwt.go:40-96,143`](backend/internal/infrastructure/auth/jwt.go)); together they skip JWT
  validation entirely and every request runs as that principal. Both are required, the principal is
  whitespace-normalised so a blank-ish value cannot arm it, and
  [lfx-mentorship#148](https://github.com/linuxfoundation/lfx-mentorship/pull/148) adds a chart
  render-time guard that refuses to template unless `allowLocalAuthBypass=true` is passed explicitly —
  checked in both `config:` and `env:`. The guard is the only thing standing between a stray values
  entry and an unauthenticated production API, so it is a cross-component contract, not a local
  convenience: **never set either key in an ArgoCD values file.**

---

## 4. External contracts

| Counterparty | Direction | Mechanism | Failure behavior |
|---|---|---|---|
| **Auth0** | inbound | PKCE for users, client-credentials for M2M, JWKS validation | Requests rejected 401 |
| **Ledger API** | Mentorship → Ledger | Hourly CronJob, `LEDGER_API_KEY` bearer token; reads mentorship-credit transactions and caches them in `program_funding_stats`. **Direct, not proxied** — `LEDGER_BASE_URL` points at the Ledger service itself (`https://ledger.dev.platform.linuxfoundation.org` in dev), and this repo has its own client ([`clients/ledger.go`](backend/internal/infrastructure/clients/ledger.go)) | Last cached values served |
| **Crowdfunding API** | Mentorship → CF → Ledger | Request-time call from `ProgramService`, Auth0 M2M token (`access:manage`); fetches categorized transactions and sponsors. **Crowdfunding is a proxy for the Ledger on this path**: `GET /v1/initiatives/{id}/transactions` is served by CF's `InitiativeService` calling the Ledger's `/transactions` ([`initiative_service.go`](https://github.com/linuxfoundation/lfx-crowdfunding/blob/main/backend/internal/service/initiative_service.go)), and `ProgramService.GetCategorizedTransactions` proxies that contract. The categorization and sponsor resolution are CF's own, which is why this does not collapse into the direct Ledger call above | `ErrUpstreamUnavailable` |
| **Snowflake** | Mentorship → SF | Fivetran Postgres connector; `fivetran_mentorship_*` dbt models repointed | Analytics-plane only — never in the serving path |
| **lfx-v2-email-service** | Mentorship → NATS `lfx.email-service.send_email` | The platform rail for all transactional email: a request/reply relay over Amazon SES, imported as `lfx-v2-email-service/pkg/api`. It accepts **pre-rendered** `html`/`text` only — no templating — so Mentorship owns and renders its own templates. Not Mandrill: that is the legacy rail and is out of scope ([linuxfoundation/lfx-self-serve#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)) | Log and continue — a failed send must not fail the business operation |
| **S3** | Mentorship → S3 | Program logos and task submissions via presigned URLs | — |
| **LFX Self Serve** | SS → Mentorship | User token against `/v1` | — |

Two Ledger upstreams, not one: the cache job and the request-time call go to **different services
with different credentials**. Do not implement against the wrong one.

**Direction of dependency matters, and it is asymmetric — state both halves:**

- **Crowdfunding does not depend on Mentorship at request time.** It consumes Mentorship data only
  through Snowflake, so nothing in *Crowdfunding's* serving path waits on this service.
- **Mentorship does depend on Crowdfunding at request time.** `GET /v1/programs/{id}/transactions`
  and `/sponsors` call Crowdfunding synchronously and return `ErrUpstreamUnavailable` when it is
  absent. Both are **public** routes, so a Crowdfunding outage degrades unauthenticated pages.

Treat Crowdfunding as a live serving-path dependency for those two endpoints, and keep it out of the
path for everything else. Funding *stats* are the safe pattern — cached by the CronJob and served
stale rather than fetched inline.

---

## 5. Data ownership

PostgreSQL (shared LFX v2 RDS) is the system of record, replacing 8 DynamoDB tables and 30 GSIs.
Full ERD in
[`docs/rewrite/02-target-architecture.md`](docs/rewrite/02-target-architecture.md#data-model-proposal-level-erd).

Cross-component notes:

- **Users are mirrored, not owned.** `users.lfid` holds the LFID
  ([`001_initial.up.sql:30`](backend/db/migrations/001_initial.up.sql)). Auth0/LF SSO remains
  authoritative for identity.
- **`programs.cii_project_id`** is the only foreign project reference on a program. It is *not* the
  Crowdfunding join key: `ProgramService.GetCategorizedTransactions` passes the mentorship
  `programID` straight through as the initiative id, so the two systems are coupled on program id.
  Worth settling deliberately rather than inheriting.
- **`program_funding_stats`** is a cache, never authoritative. It may be stale.
- **Search is Postgres full-text** (`tsvector` + GIN). Elasticsearch is dropped from scope.
- **FGA tuples are emitted through a transactional outbox.** A Postgres commit and a NATS publish
  cannot be made atomic, so each state change records a dirty-object marker in the same transaction
  and a relay re-derives the payload from current Postgres state at send time. It never replays a
  stored one: the `GenericFGAMessage` envelope carries no object version, so a stale full-state
  payload retried after a newer revocation would restore exactly the tuple that was revoked.
  fga-sync registration is PR 2 of the four-PR path in `04`.

---

## 6. Contracts that do not exist yet

Listed so that nobody builds against them. Each is agreed in principle and unowned or unbuilt in
practice — which makes them different from ordinary backlog items.

| Contract | What is missing |
|---|---|
| **The four `mentorship_*` FGA types** | Absent from `model.fga`. PR 1 of the four-PR path in [`04 §implementation path`](docs/rewrite/04-authorization-model.md); the merge gate is `tests.yaml` passing, not that the DSL parses |
| **Project-level program-admin relation** | Needs the `project` type extended *and* project-service to emit it — it cannot be durably written by this service (`04` AQ-4). The Self Serve permissions page also needs updating |
| **Program-approval global team** | The team does not exist and there is no approve endpoint. `04` AQ-8 leaves the roster owner open — "no owner re-checks that a global tuple still exists" is the operational risk on the one guard protecting publication |
| **Redaction contract for public routes** | `05` GW-8 owes the explicit field list for whatever stays unauthenticated (§3.3) |
| **Indexer registration** | Mentorship registers nothing with the indexer. If Mentorship objects should be searchable platform-wide, that work has no owner |
| **Staging and prod ArgoCD values** | Only `values/{global,dev}` exist for this service |
| **`term-status` and `task-submission-status` CronJobs** | Planned in `02-target-architecture.md`; only `program-funding-stats-sync` is specified in detail |
| **`architecture-review` label** | `lfx-self-serve` has one; the mentorship repos have none, so there is no way to flag a contract-changing PR |
