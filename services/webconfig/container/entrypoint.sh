#!/usr/bin/env bash
set -euo pipefail

rm -f /app/data/webconfig.sqlite
mkdir -p /app/data

exec /usr/local/bin/vcpe-healthd \
    --probe webconfig=/usr/local/bin/webconfig-health-probe \
    --run '/usr/local/bin/webconfig-bootstrap -f /etc/webconfig/webconfig.conf && exec /usr/local/bin/webconfig -f /etc/webconfig/webconfig.conf'