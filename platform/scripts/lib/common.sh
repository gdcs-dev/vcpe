#!/bin/bash

resolve_repo_root() {
    if [[ -n "${VCPE_INSTALL_ROOT:-}" ]]; then
        printf '%s\n' "$VCPE_INSTALL_ROOT"
        return 0
    fi

    local common_lib_dir
    common_lib_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
    cd "$common_lib_dir/../.." && pwd
}

REPO_ROOT=$(resolve_repo_root)
PLATFORM_ROOT="$REPO_ROOT/platform"
COMMON_LIB_DIR="$PLATFORM_ROOT/scripts/lib"
SERVICES_ROOT="$REPO_ROOT/services"
BNG_ROOT="$SERVICES_ROOT/bng"
RUNTIME_ROOT="$BNG_ROOT/runtime"
MV1_ROOT="$SERVICES_ROOT/mv1"
MV1_RUNTIME_ROOT="$MV1_ROOT/runtime"
WEBPA_ROOT="$SERVICES_ROOT/webpa"
XB10_ROOT="$SERVICES_ROOT/xb10"
CONFIG_ROOT=${VCPE_CONFIG_ROOT:-${XDG_CONFIG_HOME:-$HOME/.config}/vcpe}
STATE_ROOT=${VCPE_STATE_ROOT:-${XDG_STATE_HOME:-$HOME/.local/state}/vcpe}
IMAGE_NAME=${IMAGE_NAME:-ghcr.io/gdcs-dev/bng:dev}
PODMAN_COMPOSE_BIN=${PODMAN_COMPOSE_BIN:-podman-compose}
HOST_OS=$(uname -s)

ensure_dir() {
    mkdir -p "$1"
}

ensure_config_dirs() {
    ensure_dir "$CONFIG_ROOT"
    ensure_dir "$CONFIG_ROOT/profiles"
    ensure_dir "$STATE_ROOT"
}

source_env_file() {
    local env_file=$1

    [[ -f "$env_file" ]] || return 0

    # shellcheck source=/dev/null
    source "$env_file"
}

log() {
    printf '%s\n' "$*"
}

warn() {
    printf 'warning: %s\n' "$*" >&2
}

die() {
    printf 'error: %s\n' "$*" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

registry_username() {
    local username=${GHCR_USERNAME:-${GITHUB_USERNAME:-}}
    [[ -n "$username" ]] || die "set GHCR_USERNAME or GITHUB_USERNAME"
    printf '%s\n' "$username"
}

registry_token() {
    local token=${GHCR_TOKEN:-${GITHUB_TOKEN:-}}
    [[ -n "$token" ]] || die "set GHCR_TOKEN or GITHUB_TOKEN"
    printf '%s\n' "$token"
}

registry_login() {
    local registry=$1
    local username
    local token

    require_cmd podman
    [[ -n "$registry" ]] || die "registry is required"

    username=$(registry_username)
    token=$(registry_token)

    if [[ "$HOST_OS" == "Darwin" ]]; then
        printf '%s' "$token" | podman machine ssh podman login "$registry" --username "$username" --password-stdin >/dev/null
        return 0
    fi

    printf '%s' "$token" | podman login "$registry" --username "$username" --password-stdin >/dev/null
}

push_container_image() {
    local image=$1
    local registry=${image%%/*}
    local push_retry_count=${PODMAN_PUSH_RETRY_COUNT:-5}
    local push_retry_delay=${PODMAN_PUSH_RETRY_DELAY:-5s}
    local -a push_args=(
        --format v2s2
        --compression-format gzip
        --force-compression
        --retry "$push_retry_count"
        --retry-delay "$push_retry_delay"
    )

    [[ -n "$image" ]] || die "image is required"
    [[ "$registry" != "$image" ]] || die "image must include a registry prefix: $image"

    registry_login "$registry"

    if [[ "$HOST_OS" == "Darwin" ]]; then
        podman machine ssh podman push --quiet "${push_args[@]}" "$image"
        return 0
    fi

    podman push "${push_args[@]}" "$image"
}

run_root() {
    if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
        "$@"
    else
        sudo "$@"
    fi
}

run_linux_host() {
    if [[ "$HOST_OS" == "Darwin" ]]; then
        require_cmd podman
        podman machine ssh "$@"
    else
        "$@"
    fi
}

run_linux_host_root() {
    if [[ "$HOST_OS" == "Darwin" ]]; then
        require_cmd podman
        podman machine ssh sudo "$@"
    else
        run_root "$@"
    fi
}

run_linux_host_shell() {
    local command=$1
    if [[ "$HOST_OS" == "Darwin" ]]; then
        require_cmd podman
        podman machine ssh /bin/sh -lc "$command"
    else
        /bin/sh -lc "$command"
    fi
}

require_customer_id() {
    [[ -n "${1:-}" ]] || die "customer id is required"
    case "$1" in
        7|9|20)
            ;;
        *)
            die "phase-1 supports only customer ids 7, 9, and 20"
            ;;
    esac
}

ensure_runtime_root() {
    mkdir -p "$RUNTIME_ROOT"
}