<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship — Frontend

Nuxt 4 (Vue 3) SSR public site for LFX Mentorship: program discovery, program
detail, and the mentor/mentee directories. It also acts as a BFF — the Nitro
server under `server/api/` calls the Go backend and shapes responses for the UI.

Management surfaces live in
[LFX Self Serve](https://github.com/linuxfoundation/lfx-self-serve), not here.

## Requirements

- Node 22+ (the toolchain requires `node:sqlite`, unavailable on Node 20)
- pnpm

## Development

```bash
pnpm install
pnpm dev            # http://localhost:3000
```

The dev server expects the backend on `http://localhost:8080` — see the
[repository README](../README.md#local-development) for running it. Override
with `NUXT_API_BASE_URL`.

## Checks

```bash
pnpm lint
pnpm format:check
pnpm build
```

These are the same checks CI runs (`.github/workflows/ci-frontend.yml`).

## Configuration

Runtime configuration is defined in `setup/runtime-config.ts`. The values that
must be set per environment:

| Variable                        | Purpose                                                   |
| ------------------------------- | --------------------------------------------------------- |
| `NUXT_API_BASE_URL`             | Server-side base URL of the backend API                    |
| `NUXT_PUBLIC_APP_ENV`           | `development` or `production`                              |
| `NUXT_PUBLIC_APP_URL`           | Public URL of this site                                    |
| `NUXT_PUBLIC_SITE_URL`          | Sitemap and site-config URL — see below                    |
| `NUXT_PUBLIC_SELF_SERVE_URL`    | Link target for management surfaces                        |
| `NUXT_PUBLIC_CROWDFUNDING_URL`  | Link target for Crowdfunding                               |
| `NUXT_PUBLIC_INTERCOM_APP_ID`   | Intercom workspace — **required in production**            |

Two of these are not interchangeable with `NUXT_PUBLIC_APP_URL`, because the
image is built with no `NUXT_PUBLIC_*` set and only some names are re-read at
startup:

- **`NUXT_PUBLIC_SITE_URL`** — `setup/site.ts` falls back to `NUXT_PUBLIC_APP_URL`,
  but only at build time. `nuxt-site-config` re-reads `NUXT_PUBLIC_SITE_URL`
  alone at runtime, so leaving it unset serves sitemap and site URLs for
  `http://localhost:3000` even when canonical URLs are correct.
- **`NUXT_PUBLIC_INTERCOM_APP_ID`** — `setup/runtime-config.ts` derives this from
  the *build-time* `appEnv`, baking in the development workspace id. It is not
  re-derived from `NUXT_PUBLIC_APP_ENV`, so production must set it explicitly;
  the chart refuses to render otherwise.

## Deployment

Built as a container image (`Dockerfile`) and deployed via
`charts/lfx-mentorship-frontend`. See the [repository README](../README.md#deployment).
