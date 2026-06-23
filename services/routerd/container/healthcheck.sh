#!/bin/bash
set -euo pipefail

ROUTERD_SOCKET=${ROUTERD_SOCKET:-/run/routerd/routerd.sock}
ROUTERD_BIN_DIR=${ROUTERD_BIN_DIR:-/workspace/target/release}
ROUTERCTL_BIN=${ROUTERCTL_BIN:-$ROUTERD_BIN_DIR/routerctl}

[[ -S "$ROUTERD_SOCKET" ]]
"$ROUTERCTL_BIN" --socket "$ROUTERD_SOCKET" status >/dev/null
ip link show brlan0 >/dev/null
