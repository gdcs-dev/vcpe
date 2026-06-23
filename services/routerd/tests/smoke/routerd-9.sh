#!/bin/bash
set -euo pipefail

podman ps --filter name=routerd-9 --format '{{.Names}}' | grep -q '^routerd-9$'
podman exec routerd-9 ip link show brlan0 >/dev/null
podman exec routerd-9 ip link show erouter0.1081 >/dev/null
podman exec routerd-9 /workspace/target/release/routerctl status >/dev/null
podman exec routerd-9 ping -c 1 -W 3 -I erouter0.1081 10.178.200.1 >/dev/null
