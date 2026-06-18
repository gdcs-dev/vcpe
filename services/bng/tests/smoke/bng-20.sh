#!/bin/bash
set -euo pipefail

podman ps --filter name=bng-20 --format '{{.Names}}' | grep -q '^bng-20$'
podman exec bng-20 ip link show eth1.100 >/dev/null
podman exec bng-20 ss -lnt | grep -q '10.120.200.1:80'
podman exec bng-20 ss -lnt | grep -q '2001:dae:20:1::129]:80'
podman exec bng-20 ping -c 1 -W 3 1.1.1.1 >/dev/null