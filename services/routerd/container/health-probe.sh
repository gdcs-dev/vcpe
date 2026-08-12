#!/bin/sh
set -eu

routerd_socket=${ROUTERD_SOCKET:-/run/routerd/routerd.sock}
routerd_bin_dir=${ROUTERD_BIN_DIR:-/workspace/target/release}
routerctl_bin=${ROUTERCTL_BIN:-$routerd_bin_dir/routerctl}

test -S "$routerd_socket"
"$routerctl_bin" --socket "$routerd_socket" status >/dev/null
ip link show brlan0 >/dev/null