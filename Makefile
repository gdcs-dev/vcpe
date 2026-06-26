SHELL := /bin/bash

VCPE_BIN ?= controlplane/bin/vcpe
VCPE_BIN_REL := $(patsubst controlplane/%,%,$(VCPE_BIN))
MANIFEST ?=
CUSTOMER ?= 7

.PHONY: help init build up down status logs-bng logs-webpa profile-list smoke-go smoke-services smoke-controlplane release-gate clean

help:
	@echo "vCPE developer helpers (direct vcpe command wrappers)"
	@echo ""
	@echo "Core"
	@echo "  make init            # $(VCPE_BIN) init"
	@echo "  make build           # build controlplane Go binary"
	@echo "  make up MANIFEST=path/to/manifest.yaml"
	@echo "  make status [CUSTOMER=7]"
	@echo "  make down CUSTOMER=7"
	@echo ""
	@echo "Logs"
	@echo "  make logs-bng        # $(VCPE_BIN) service bng logs --customer $(CUSTOMER)"
	@echo "  make logs-webpa      # $(VCPE_BIN) service webpa logs"
	@echo ""
	@echo "Profiles"
	@echo "  make profile-list    # $(VCPE_BIN) profile list"
	@echo ""
	@echo "Smoke"
	@echo "  make smoke-go        # primary vcpe command smoke"
	@echo "  make smoke-services  # direct vcpe service namespace smoke"
	@echo "  make smoke-controlplane # podman integration smoke (when available)"
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
	$(VCPE_BIN) status --customer "$(CUSTOMER)"

down:
	$(VCPE_BIN) down --customer "$(CUSTOMER)" --force

logs-bng:
	$(VCPE_BIN) service bng logs --customer "$(CUSTOMER)"

logs-webpa:
	$(VCPE_BIN) service webpa logs

profile-list:
	$(VCPE_BIN) profile list

smoke-go:
	./tests/smoke/vcpe-primary-status.sh

smoke-services:
	./tests/smoke/vcpe-service-coverage.sh

smoke-controlplane:
	./tests/smoke/controlplane-bng-7.sh
	./tests/smoke/controlplane-bng-20.sh

release-gate:
	cd controlplane && go test ./...
	./tests/smoke/vcpe-primary-status.sh
	./tests/smoke/vcpe-service-coverage.sh
	./tests/smoke/controlplane-bng-7.sh
	./tests/smoke/controlplane-bng-20.sh

clean:
	@echo "No build artifacts are managed by this Makefile."
