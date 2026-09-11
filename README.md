<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship

The LFX Mentorship platform — a rewrite of the legacy CommunityBridge Mentorship system ([jobspring](https://github.com/linuxfoundation/jobspring) + [lfx-mentorship-upgrade](https://github.com/linuxfoundation/lfx-mentorship-upgrade)), following the architecture proven by the [Crowdfunding rewrite](https://github.com/linuxfoundation/lfx-crowdfunding).

## Status

**In implementation.** The architecture proposal is approved and the backend and
public site are being built:

- Proposal: [docs/rewrite/](docs/rewrite/) — [current system](docs/rewrite/01-current-system.md), [target architecture](docs/rewrite/02-target-architecture.md), [migration plan](docs/rewrite/03-migration-plan.md)
- Tracking ticket: [linuxfoundation/lfx-self-serve#1526](https://github.com/linuxfoundation/lfx-self-serve/issues/1526)
- Milestone 1 epic: [linuxfoundation/lfx-self-serve#1477](https://github.com/linuxfoundation/lfx-self-serve/issues/1477) (Mentee public site)

## Architecture

- **Backend**: Go (Chi), PostgreSQL on the shared LFX v2 RDS, layered domain/service/handler/infrastructure design
- **Frontend**: Nuxt 4 (Vue 3) SSR public site — discovery, program detail, apply flow; management surfaces live in [LFX Self Serve](https://github.com/linuxfoundation/lfx-self-serve)
- **Deployment**: Kubernetes (LFX v2 cluster), Helm charts, ArgoCD GitOps
- **Auth**: Auth0 (OAuth2 PKCE for users, M2M for services), validated in-service today

Authorization is expected to move to the platform's Heimdall/OpenFGA edge — see
[linuxfoundation/lfx-mentorship#119](https://github.com/linuxfoundation/lfx-mentorship/pull/119).

## Local development

Start a local Postgres first (from the repo root) — it listens on **5433** to avoid
clashing with a system Postgres on 5432:

```bash
docker compose up -d     # Postgres only; `down -v` wipes the volume
```

Then, from `backend/` — with `DATABASE_DSN` in `backend/.env` pointing at it:

```bash
make db-migrate   # apply migrations (refuses any non-localhost DATABASE_DSN)
make run          # start the API on :8080
make test         # unit tests
make lint
```

Running `cmd/program-funding-stats-sync` against this database populates the
program funding stats table locally.

Frontend (from `frontend/`) — requires Node 22+ and pnpm:

```bash
pnpm install
pnpm dev          # http://localhost:3000
pnpm lint
pnpm format:check
```

## Deployment

Both services ship as container images and Helm charts:

| Component | Image | Chart |
|---|---|---|
| Backend | `ghcr.io/linuxfoundation/lfx-mentorship-backend` | `backend/charts/lfx-mentorship-backend` |
| Frontend | `ghcr.io/linuxfoundation/lfx-mentorship-frontend` | `frontend/charts/lfx-mentorship-frontend` |

A push to `main` publishes images tagged `development`, which the dev
environment tracks automatically. A `v*.*.*` tag publishes semver-tagged images
plus signed OCI charts; staging and prod pin those chart versions, so promoting
a release is a version-bump PR in
[lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd).

Database migrations run automatically as a Helm `pre-install`/`pre-upgrade` hook
(`cmd/migrate`), so schema changes land before new pods serve traffic.

## License

This project is licensed under the [MIT License](LICENSE).
