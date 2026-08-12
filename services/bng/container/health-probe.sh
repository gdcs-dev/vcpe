#!/bin/sh
set -eu

pgrep -x apache2 >/dev/null
pgrep -x dnsmasq >/dev/null
pgrep -x mosquitto >/dev/null
ss -lnt | grep -q ':80 '
ss -lnt | grep -q ':1883 '