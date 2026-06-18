#!/bin/bash
set -euo pipefail

source /etc/service-interfaces.env

mkdir -p /var/log/bng
mkdir -p /var/lib/dhcp
touch /var/log/bng/services.log
touch /var/lib/dhcp/dhcpd.leases /var/lib/dhcp/dhcpd6.leases

dnsmasq --keep-in-foreground --conf-file=/etc/dnsmasq.conf &
apachectl start
mosquitto -d
ntpd -g -u ntp:ntp || true
dhcpd -4 -cf /etc/dhcp/dhcpd.conf ${DHCP4_INTERFACES} || true
dhcpd -6 -cf /etc/dhcp/dhcpd6.conf ${DHCP6_INTERFACES} || true
radvd -C /etc/radvd.conf -m logfile -l syslog || true

echo "services started" >> /var/log/bng/services.log
exec tail -f /var/log/bng/services.log