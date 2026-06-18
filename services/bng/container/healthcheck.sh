#!/bin/bash
set -euo pipefail

pgrep -x apache2 >/dev/null
pgrep -x mosquitto >/dev/null
ss -lnt | grep -q ':80 '