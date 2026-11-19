# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

# Build stage
FROM cgr.dev/chainguard/go:latest-dev@sha256:12b02b9f84197e22786e72eb18a584c3f5f4603710cec5b7b951e351504e1d4f AS builder

WORKDIR /build

# Cache dependencies separately from source code.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/mentorship-api ./cmd/mentorship-api \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/program-funding-stats-sync ./cmd/program-funding-stats-sync \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/outbox-repair ./cmd/outbox-repair \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/prerequisite-reminder ./cmd/prerequisite-reminder \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/approver-admin ./cmd/approver-admin \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/user-backfill ./cmd/user-backfill \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/term-status ./cmd/term-status \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" \
    -o /build/program-project-sync ./cmd/program-project-sync

# Runtime stage
# Distroless static image minimizes runtime attack surface while retaining CA certs.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

WORKDIR /app
COPY --from=builder /build/mentorship-api ./mentorship-api
COPY --from=builder /build/program-funding-stats-sync ./program-funding-stats-sync
COPY --from=builder /build/migrate ./migrate
COPY --from=builder /build/outbox-repair ./outbox-repair
COPY --from=builder /build/prerequisite-reminder ./prerequisite-reminder
COPY --from=builder /build/approver-admin ./approver-admin
COPY --from=builder /build/user-backfill ./user-backfill
COPY --from=builder /build/term-status ./term-status
COPY --from=builder /build/program-project-sync ./program-project-sync

EXPOSE 8080

ENTRYPOINT ["/app/mentorship-api"]
