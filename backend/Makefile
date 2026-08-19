# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

BINARY      := mentorship-api
MODULE      := github.com/linuxfoundation/lfx-v2-mentorship-service
CMD         := ./cmd/mentorship-api
IMAGE_NAME  := lfx-v2-mentorship-service
IMAGE_TAG   ?= dev
DOCKER_REG  ?= ghcr.io/linuxfoundation

# kind / local deploy
KIND_CLUSTER     ?= mentorship-dev
NAMESPACE        ?= lfx-v2-mentorship-service
RELEASE_NAME     ?= mentorship
CHART_DIR        ?= charts/lfx-mentorship-backend
DEPLOYMENT_NAME  ?= $(RELEASE_NAME)-lfx-mentorship-backend
DEPLOY_TAG       ?= dev-$(shell date +%Y%m%d%H%M%S)

.PHONY: all clean deps build run test fmt lint license-check \
        docker-build docker-run helm-install deploy-kind db-migrate

all: build

## deps: download Go module dependencies
deps:
	go mod download

## build: compile the binary
build: deps
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/$(BINARY) $(CMD)

## run: run the service locally (reads DATABASE_DSN and other vars from .env)
run:
	@set -a && . ./.env && set +a && \
	  DISABLED_MOCK_LOCAL_PRINCIPAL=localdev \
	  ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS=true \
	  go run $(CMD)

## test: run all unit tests
test:
	go test -race -count=1 ./...

## fmt: format all Go source files
fmt:
	gofmt -w .
	goimports -w .

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## license-check: verify all .go files have license headers
license-check:
	@missing=$$(grep -rL "SPDX-License-Identifier: MIT" --include="*.go" .); \
	if [ -n "$$missing" ]; then \
	  echo "Missing license header in:"; \
	  echo "$$missing"; \
	  exit 1; \
	fi
	@echo "All .go files have license headers."

## clean: remove build artefacts
clean:
	rm -rf bin/

## db-migrate: run pending SQL migrations against the local database
db-migrate:
	@echo "$$DATABASE_DSN" | grep -qE "localhost|127\.0\.0\.1" || \
		{ echo "ERROR: db-migrate refused — DATABASE_DSN does not point to localhost"; exit 1; }
	@for f in db/migrations/*.up.sql; do \
	  echo "Applying $$f..."; \
	  psql "$$DATABASE_DSN" -f "$$f"; \
	done

## docker-build: build the Docker image
docker-build:
	docker build -t $(DOCKER_REG)/$(IMAGE_NAME):$(IMAGE_TAG) .

## docker-run: run the Docker image locally
docker-run: docker-build
	docker run --rm --env-file .env -p 8080:8080 $(DOCKER_REG)/$(IMAGE_NAME):$(IMAGE_TAG)

## helm-install: install / upgrade the Helm chart in the current k8s context
helm-install:
	helm upgrade --install $(RELEASE_NAME) $(CHART_DIR) \
	  --namespace lfx --create-namespace \
	  --values $(CHART_DIR)/values.yaml

## deploy-kind: build image, load into kind, helm upgrade, wait, port-forward
##   Requires: kind, kubectl, helm, docker, .env file
##   Override cluster:   make deploy-kind KIND_CLUSTER=my-cluster
##   Override namespace: make deploy-kind NAMESPACE=my-ns
deploy-kind:
	@echo "==> Ensuring kind cluster '$(KIND_CLUSTER)' exists..."
	@kind get clusters | grep -q '^$(KIND_CLUSTER)$$' || kind create cluster --name $(KIND_CLUSTER)
	@echo "==> Switching kubectl context to kind-$(KIND_CLUSTER)..."
	@kubectl config use-context kind-$(KIND_CLUSTER)
	@echo "==> Building Docker image $(IMAGE_NAME):$(DEPLOY_TAG)..."
	@docker build -t $(IMAGE_NAME):$(DEPLOY_TAG) .
	@echo "==> Loading image into kind cluster..."
	@kind load docker-image $(IMAGE_NAME):$(DEPLOY_TAG) --name $(KIND_CLUSTER)
	@echo "==> Ensuring namespace '$(NAMESPACE)' exists..."
	@kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f - >/dev/null
	@echo "==> Seeding k8s Secret from .env (placeholder — ESO not present in kind)..."
	@set -a; . ./.env; set +a; \
		KIND_DSN=$$(echo "$$DATABASE_DSN" | sed 's/localhost/host.docker.internal/g; s/127\.0\.0\.1/host.docker.internal/g'); \
		kubectl -n $(NAMESPACE) create secret generic lfx-mentorship-backend-secrets \
			--from-literal=DATABASE_DSN="$$KIND_DSN" \
			--dry-run=client -o yaml | kubectl apply -f - >/dev/null
	@echo "==> Running helm upgrade --install..."
	@helm upgrade --install $(RELEASE_NAME) $(CHART_DIR) \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMAGE_NAME) \
		--set image.tag=$(DEPLOY_TAG) \
		--set image.pullPolicy=IfNotPresent \
		--set autoscaling.enabled=false \
		--set createPlaceholderSecret=false \
		--set-string config.DISABLED_MOCK_LOCAL_PRINCIPAL=local-dev-user
	@echo "==> Waiting for rollout..."
	@kubectl -n $(NAMESPACE) rollout status deploy/$(DEPLOYMENT_NAME) --timeout=180s
	@echo "==> Setting up port-forward on :18090 -> svc :80..."
	@existing_pid=$$(lsof -tiTCP:18090 -sTCP:LISTEN 2>/dev/null || true); \
	if [ -n "$$existing_pid" ]; then \
		echo "Stopping existing listener on 18090 (PID $$existing_pid)..."; \
		kill $$existing_pid 2>/dev/null || true; \
	fi
	@nohup kubectl -n $(NAMESPACE) port-forward svc/$(DEPLOYMENT_NAME) 18090:80 \
		>/tmp/$(RELEASE_NAME)-port-forward.log 2>&1 &
	@sleep 2
	@curl -sf http://127.0.0.1:18090/livez >/dev/null \
		&& echo "==> Deployment ready. Local access: http://127.0.0.1:18090" \
		|| echo "==> Port-forward started; verify with: tail -f /tmp/$(RELEASE_NAME)-port-forward.log"
