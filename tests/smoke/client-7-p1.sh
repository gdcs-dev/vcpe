#!/bin/bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

"$REPO_ROOT/scripts/client" up mv1-r21-7-p1 >/dev/null

podman ps --filter name=client-mv1-r21-7-p1 --format '{{.Names}}' | grep -q '^client-mv1-r21-7-p1$'
podman inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' client-mv1-r21-7-p1 | grep -q '^lan-p1-7$'
podman exec client-mv1-r21-7-p1 /bin/sh -c 'test -d /sys/class/net/eth0'
podman exec client-mv1-r21-7-p1 /bin/sh -c 'ip -4 addr show dev eth0 | grep -q "192.168.0.115/24"'
podman exec client-mv1-r21-7-p1 /bin/sh -c 'ip -6 addr show dev eth0 | grep -q "3001:dae:0:f000::da/64"'
podman exec client-mv1-r21-7-p1 /bin/sh -c 'ip route show default | grep -q "default via 192.168.0.1 dev eth0"'