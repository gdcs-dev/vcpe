SHELL := /bin/bash

VCPE_BIN ?= controlplane/bin/vcpe
VCPE_BIN_REL := $(patsubst controlplane/%,%,$(VCPE_BIN))
MANIFEST ?=
NAME ?= example

.PHONY: help init build up down status logs-bng logs-webpa smoke-go smoke-services smoke-controlplane release-gate clean

help:
	@echo "vCPE developer helpers (direct vcpe command wrappers)"
	@echo ""
	@echo "Core"
	@echo "  make init            # $(VCPE_BIN) init"
	@echo "  make build           # build controlplane Go binary"
	@echo "  make up MANIFEST=path/to/manifest.yaml"
	@echo "  make status [NAME=example]"
	@echo "  make down [NAME=example]"
	@echo ""
	@echo "Logs"
	@echo "  make logs-bng        # $(VCPE_BIN) service bng logs --name $(NAME)"
	@echo "  make logs-webpa      # $(VCPE_BIN) service webpa logs"
	@echo ""
	@echo "Smoke"
	@echo "  make release-gate    # required pre-ship checks"

init:
	$(VCPE_BIN) init

build:
	@mkdir -p "$(dir $(VCPE_BIN))"
	cd controlplane && mkdir -p "$(dir $(VCPE_BIN_REL))" && go build -o "$(VCPE_BIN_REL)" ./cmd/vcpe

up:
	@test -n "$(MANIFEST)" || (echo "MANIFEST is required" >&2; exit 1)
	$(VCPE_BIN) up --manifest "$(MANIFEST)"

status:
	$(VCPE_BIN) status --name "$(NAME)"

down:
	$(VCPE_BIN) down --name "$(NAME)" --force

logs-bng:
	$(VCPE_BIN) service bng logs --name "$(NAME)"

logs-webpa:
	$(VCPE_BIN) service webpa logs

release-gate:
	cd controlplane && go test ./...

clean:
	@echo "No build artifacts are managed by this Makefile."
