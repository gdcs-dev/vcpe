#!/bin/bash
set -euo pipefail

podman ps --filter name=bng-9 --format '{{.Names}}' | grep -q '^bng-9$'
podman exec bng-9 ip link show eth1.1081 >/dev/null
podman exec bng-9 ip link show eth1.881 >/dev/null
podman exec bng-9 ip link show eth1.981 >/dev/null
podman exec bng-9 ss -lnt | grep -q '10.178.200.1:80'
podman exec bng-9 ss -lnt | grep -q '2001:dbe:0:1::129]:80'
podman exec bng-9 ping -c 1 -W 3 1.1.1.1 >/dev/null