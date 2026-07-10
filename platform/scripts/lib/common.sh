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

log() {
    printf '%s\n' "$*"
}

die() {
    printf 'error: %s\n' "$*" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}