#!/bin/bash
set -euo pipefail

podman ps --filter name=routerd-20 --format '{{.Names}}' | grep -q '^routerd-20$'
podman exec routerd-20 ip link show brlan0 >/dev/null
podman exec routerd-20 ip link show erouter0.100 >/dev/null
podman exec routerd-20 /workspace/target/release/routerctl status >/dev/null
podman exec routerd-20 ping -c 1 -W 3 -I erouter0.100 10.120.200.1 >/dev/null
