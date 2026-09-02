# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

# Build stage
FROM cgr.dev/chainguard/go:latest-dev@sha256:21e9c8e3133f0aad37910aa30f437e31958ed15cd4494957e04e5d5e4bab9f69 AS builder

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
    -o /build/migrate ./cmd/migrate

# Runtime stage
# Distroless static image minimizes runtime attack surface while retaining CA certs.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

WORKDIR /app
COPY --from=builder /build/mentorship-api ./mentorship-api
COPY --from=builder /build/program-funding-stats-sync ./program-funding-stats-sync
COPY --from=builder /build/migrate ./migrate

EXPOSE 8080

ENTRYPOINT ["/app/mentorship-api"]
