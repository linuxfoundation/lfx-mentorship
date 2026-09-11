# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

# ── Build stage ──────────────────────────────────────────────────────────────
# Digest-pinned (multi-arch index) so rebuilds are reproducible.
FROM node:22-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS builder

# Enable corepack and activate the pnpm version pinned in package.json's
# packageManager field, so the image resolves dependencies with the same pnpm
# the lockfile and CI were verified against rather than Corepack's latest.
RUN corepack enable && corepack install --global pnpm@11.1.2

WORKDIR /build

# Cache the pnpm store separately from source — rebuild only when the manifest
# changes. pnpm-workspace.yaml carries allowBuilds settings that affect
# resolution, so it belongs in this layer alongside the lockfile.
COPY pnpm-lock.yaml package.json pnpm-workspace.yaml ./
RUN pnpm fetch

COPY . .

RUN pnpm install --frozen-lockfile --prefer-offline && pnpm build

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM node:22-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS runtime

RUN addgroup -S appgroup && adduser -S appuser -G appgroup -u 65532

WORKDIR /app

# Nuxt 4 output: .output/server (Node server) + .output/public (static assets)
COPY --from=builder --chown=appuser:appgroup /build/.output/ ./

USER appuser

EXPOSE 3000

ENV NODE_ENV=production
ENV NITRO_HOST=0.0.0.0
ENV NITRO_PORT=3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:${NITRO_PORT:-3000}/api/health || exit 1

ENTRYPOINT ["node", "server/index.mjs"]
