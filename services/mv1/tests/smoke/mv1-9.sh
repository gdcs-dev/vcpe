#!/bin/bash
set -euo pipefail

podman ps --filter name=mv1-r21-9 --format '{{.Names}}' | grep -q '^mv1-r21-9$'
podman exec mv1-r21-9 ip link show brlan0 >/dev/null
podman exec mv1-r21-9 ip link show erouter0.1081 >/dev/null
podman exec mv1-r21-9 ping -c 1 -W 3 -I erouter0.1081 10.178.200.1 >/dev/null