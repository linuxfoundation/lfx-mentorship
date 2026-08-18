<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship

The LFX Mentorship platform — a rewrite of the legacy CommunityBridge Mentorship system ([jobspring](https://github.com/linuxfoundation/jobspring) + [lfx-mentorship-upgrade](https://github.com/linuxfoundation/lfx-mentorship-upgrade)), following the architecture proven by the [Crowdfunding rewrite](https://github.com/linuxfoundation/lfx-crowdfunding).

## Status

**Pre-implementation.** The architecture proposal is under review:

- Proposal: [docs/rewrite/](docs/rewrite/) — [current system](docs/rewrite/01-current-system.md), [target architecture](docs/rewrite/02-target-architecture.md), [migration plan](docs/rewrite/03-migration-plan.md)
- Tracking ticket: [linuxfoundation/lfx-self-serve#1526](https://github.com/linuxfoundation/lfx-self-serve/issues/1526)
- Milestone 1 epic: [linuxfoundation/lfx-self-serve#1477](https://github.com/linuxfoundation/lfx-self-serve/issues/1477) (Mentee public site)

Implementation begins here once the proposal is approved.

## Planned architecture

- **Backend**: Go (Chi), PostgreSQL (`mentorship` schema on the shared LFX v2 RDS), layered domain/service/handler/infrastructure design
- **Frontend**: Nuxt 4 (Vue 3) SSR public site — discovery, program detail, apply flow; management surfaces live in [LFX Self Serve](https://github.com/linuxfoundation/lfx-self-serve)
- **Deployment**: Kubernetes (LFX v2 cluster), Helm charts, ArgoCD GitOps
- **Auth**: Auth0 (OAuth2 PKCE for users, M2M for services)

See the proposal PR above for full details.

## License

This project is licensed under the [MIT License](LICENSE).
