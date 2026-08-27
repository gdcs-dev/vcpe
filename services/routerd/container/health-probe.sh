#!/bin/sh
set -eu

routerd_socket_dir=${ROUTERD_SOCKET_DIR:-/run/routerd}
routerd_bin_dir=${ROUTERD_BIN_DIR:-/usr/bin}
routerctl_bin=${ROUTERCTL_BIN:-$routerd_bin_dir/routerctl}

"$routerctl_bin" --socket-dir "$routerd_socket_dir" status >/dev/null
