#!/bin/bash
set -euo pipefail

podman ps --filter name=mv1-r21-20 --format '{{.Names}}' | grep -q '^mv1-r21-20$'
podman exec mv1-r21-20 ip link show brlan0 >/dev/null
podman exec mv1-r21-20 ip link show erouter0.100 >/dev/null
podman exec mv1-r21-20 ping -c 1 -W 3 -I erouter0.100 10.120.200.1 >/dev/null