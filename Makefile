# ─────────────────────────────────────────────────────────────────────────────
# Timescale Writer — Makefile
#
# Usage:
#   make build          Build the Docker image locally
#   make tag            Tag the image for Docker Hub
#   make push           Push the tagged image to Docker Hub
#   make release        build + tag + push in one shot
#   make run            Run the container locally (reads .env)
#   make test           Run Go unit tests
#   make clean          Remove local image
#
# Required env vars (set in your shell or CI):
#   DOCKER_USER         Your Docker Hub username  (e.g. 120m4n)
#   DOCKER_PASS         Your Docker Hub password / access token
#
# Optional overrides:
#   IMAGE_NAME          Defaults to timescale-writer
#   TAG                 Defaults to git short SHA or "latest" if no git
# ─────────────────────────────────────────────────────────────────────────────

# ── Identity ─────────────────────────────────────────────────────────────────
DOCKER_USER  ?= electrosoftware
IMAGE_NAME   ?= timescale-writer
TAG          ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)

FULL_IMAGE   := $(DOCKER_USER)/$(IMAGE_NAME)
VERSIONED    := $(FULL_IMAGE):$(TAG)
LATEST       := $(FULL_IMAGE):latest

# ── Go ───────────────────────────────────────────────────────────────────────
GO           := go
GOFLAGS      := -v

# ─────────────────────────────────────────────────────────────────────────────
.PHONY: all build tag push release run test clean info

all: release

## info — print resolved variables
info:
	@echo "Image   : $(VERSIONED)"
	@echo "Latest  : $(LATEST)"
	@echo "User    : $(DOCKER_USER)"

## build — build the Docker image
build:
	@echo "▶  Building $(VERSIONED) …"
	docker build \
		--platform linux/amd64 \
		--label "org.opencontainers.image.revision=$(TAG)" \
		--label "org.opencontainers.image.created=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)" \
		-t $(VERSIONED) \
		-t $(LATEST) \
		-f Dockerfile .Dockerfile.timescale
	@echo "✅ Image built: $(VERSIONED)"

## tag — (re)tag the local image; useful if you built with a custom name
tag:
	@echo "▶  Tagging $(VERSIONED) → $(LATEST) …"
	docker tag $(VERSIONED) $(LATEST)
	@echo "✅ Tagged"

## push — push versioned + latest tags to Docker Hub
push:
	@echo "▶  Logging in to Docker Hub as $(DOCKER_USER) …"
	@echo "$(DOCKER_PASS)" | docker login -u "$(DOCKER_USER)" --password-stdin
	@echo "▶  Pushing $(VERSIONED) …"
	docker push $(VERSIONED)
	@echo "▶  Pushing $(LATEST) …"
	docker push $(LATEST)
	@echo "✅ Pushed: $(VERSIONED) and $(LATEST)"

## release — build + push in one shot
release: build push

## run — run the container locally using the .env file
run:
	@echo "▶  Running $(LATEST) …"
	docker run --rm \
		--env-file .env \
		--name $(IMAGE_NAME) \
		$(LATEST)

## test — run Go unit tests
test:
	@echo "▶  Running tests …"
	$(GO) test $(GOFLAGS) ./internal/...

## clean — remove local Docker image
clean:
	@echo "▶  Removing local images …"
	-docker rmi $(VERSIONED) $(LATEST) 2>/dev/null || true
	@echo "✅ Cleaned"
