<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repo hosts the LFX Mentorship platform — a Kubernetes-native rewrite of the legacy serverless Mentorship system ([jobspring](https://github.com/linuxfoundation/jobspring) backend + [lfx-mentorship-upgrade](https://github.com/linuxfoundation/lfx-mentorship-upgrade) frontend).

**Read `docs/rewrite/` before making changes** — it is the approved architecture:

- `docs/rewrite/01-current-system.md` — the legacy platform being replaced
- `docs/rewrite/02-target-architecture.md` — target architecture (start here)
- `docs/rewrite/03-migration-plan.md` — migration phases and open questions

## Conventions

- **Follow [lfx-crowdfunding](https://github.com/linuxfoundation/lfx-crowdfunding)** — it is the explicit template for this rewrite: same monorepo layout (`backend/` Go + Chi, `frontend/` Nuxt 4 BFF), same layered backend design (domain/service/handler/infrastructure), same deployment model (Helm charts in-repo, ArgoCD GitOps). When unsure how to structure something, look at how lfx-crowdfunding does it.
- **License headers**: every file needs the MIT/SPDX header (`Copyright The Linux Foundation and each contributor to LFX.` / `SPDX-License-Identifier: MIT`) — enforced by CI.
- **DCO**: sign off every commit (`git commit --signoff`) — enforced by CI.
- **Scope**: the goal is feature parity with the legacy platform. Scope exclusions (employer portal, Elasticsearch, SES, and others) are listed in `docs/rewrite/02-target-architecture.md` — do not reintroduce them.

## Status

Pre-implementation. This file intentionally omits build commands and directory structure — add them here as they become real. Tracking: [linuxfoundation/lfx-self-serve#1526](https://github.com/linuxfoundation/lfx-self-serve/issues/1526).
