#!/bin/bash
set -euo pipefail

podman ps --filter name=bng-7 --format '{{.Names}}' | grep -q '^bng-7$'
podman exec bng-7 ss -lnt | grep -q ':80 '
podman exec bng-7 ping -c 1 -W 3 1.1.1.1 >/dev/null