#!/bin/bash

WEBPA_PODMAN_LIB_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

build_webpa_image() {
    require_cmd podman
    "$REPO_ROOT/scripts/stage-runtime-init-binaries" webpa
    podman build -t "${WEBPA_IMAGE_NAME:-ghcr.io/gdcs-dev/webpa:dev}" "$WEBPA_ROOT"
}

push_webpa_image() {
    push_container_image "${WEBPA_IMAGE_NAME:-ghcr.io/gdcs-dev/webpa:dev}"
}

_webpa_compose_env() {
    local env_file="$WEBPA_ROOT/runtime/compose.env"
    mkdir -p "$(dirname "$env_file")"
    printf 'WEBPA_IMAGE_NAME=%s\n' "${WEBPA_IMAGE_NAME:-ghcr.io/gdcs-dev/webpa:dev}" >"$env_file"
    printf '%s\n' "$env_file"
}

compose_up_webpa() {
    local env_file
    env_file=$(_webpa_compose_env)
    (cd "$WEBPA_ROOT" && "$PODMAN_COMPOSE_BIN" -p podman-webpa --env-file "$env_file" up -d)
}

compose_down_webpa() {
    local env_file
    env_file=$(_webpa_compose_env)
    (cd "$WEBPA_ROOT" && "$PODMAN_COMPOSE_BIN" -p podman-webpa --env-file "$env_file" down)
}

container_status_webpa() {
    podman ps -a --filter "name=webpa"
}

container_logs_webpa() {
    podman logs webpa
}
