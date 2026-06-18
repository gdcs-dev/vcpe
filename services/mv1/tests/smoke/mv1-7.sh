#!/bin/bash
set -euo pipefail

podman ps --filter name=mv1-r21-7 --format '{{.Names}}' | grep -q '^mv1-r21-7$'
podman exec mv1-r21-7 ip link show brlan0 >/dev/null
podman exec mv1-r21-7 ping -c 1 -W 3 -I erouter0 10.107.200.1 >/dev/null
podman exec mv1-r21-7 ping -c 1 -W 3 -I wan0 10.107.201.1 >/dev/null