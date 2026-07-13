#!/bin/bash

# Wait for an IP before starting
IPADDR="$(ifconfig | grep -A 1 'erouter0' | grep 'inet ' | awk '{print $2}')"
while [ -z "$IPADDR" ]
do
    sleep 2
    IPADDR="$(ifconfig | grep -A 1 'erouter0' | grep 'inet ' | awk '{print $2}')"
done

BaseMACAddress=$(cat /sys/class/net/erouter0/address)
SerialNumber=${BaseMACAddress//:/}
Manufacturer="Comcast"
ModelName="VCPE"
Interface="erouter0"
BootTime=$(awk '/btime/{print $2}' /proc/stat)

/usr/bin/parodus \
    --hw-model=$ModelName \
    --hw-serial-number="$SerialNumber" \
    --hw-manufacturer=$Manufacturer \
    --hw-last-reboot-reason=unknown \
    --fw-name=fakefirmware \
    --boot-time="$BootTime" \
    --hw-mac="$BaseMACAddress" \
    --webpa-ping-time=180 \
    --webpa-interface-used=$Interface \
    --webpa-url=http://talaria:6200 \
    --webpa-backoff-max=60  \
    --parodus-local-url=tcp://127.0.0.1:6666 \
    --partner-id=comcast \
    --force-ipv4  \
    --token-server-url=http://themis:6501/issue