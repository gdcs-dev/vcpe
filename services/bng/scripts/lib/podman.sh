#!/bin/bash

PODMAN_LIB_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

build_image() {
    require_cmd podman
    "$REPO_ROOT/scripts/stage-runtime-init-binaries" bng
    podman build -t "$IMAGE_NAME" "$BNG_ROOT"
}

push_image() {
    push_container_image "$IMAGE_NAME"
}

compose_project_name() {
    local customer_id=$1
    printf 'podman-bng-%s\n' "$customer_id"
}

compose_env_file() {
    local customer_id=$1
    local env_file="$RUNTIME_ROOT/$customer_id/compose.env"
    python3 "$PODMAN_LIB_DIR/customer_config.py" compose-env "$BNG_ROOT" "$customer_id" "$IMAGE_NAME" "$RUNTIME_ROOT" >"$env_file"
    printf '%s\n' "$env_file"
}

compose_up() {
    local customer_id=$1
    local env_file
    local project_name
    env_file=$(compose_env_file "$customer_id")
    project_name=$(compose_project_name "$customer_id")
    (cd "$BNG_ROOT" && "$PODMAN_COMPOSE_BIN" -p "$project_name" --env-file "$env_file" up -d)
}

compose_down() {
    local customer_id=$1
    local env_file
    local project_name
    env_file=$(compose_env_file "$customer_id")
    project_name=$(compose_project_name "$customer_id")
    (cd "$BNG_ROOT" && "$PODMAN_COMPOSE_BIN" -p "$project_name" --env-file "$env_file" down)
}

container_status() {
    local customer_id=$1
    podman ps -a --filter "name=bng-${customer_id}"
}

container_logs() {
    local customer_id=$1
    podman logs "bng-${customer_id}"
}