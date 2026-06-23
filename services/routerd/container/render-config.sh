#!/bin/bash
set -euo pipefail

# Render a routerd desired-state config from the gateway env vars used by
# mv1/xb10 (BRLAN0_*, WAN0_*, EROUTER0_*). Output goes to stdout as JSON.

erouter_iface=erouter0
vlans_json='[]'
if [[ -n "${EROUTER0_VLAN:-}" ]]; then
    erouter_iface="erouter0.${EROUTER0_VLAN}"
    vlans_json=$(cat <<JSON
[ { "parent": "erouter0", "vlan_id": ${EROUTER0_VLAN} } ]
JSON
)
fi

managed_json='[
    { "name": "eth0" },
    { "name": "eth1" },
    { "name": "eth2" },
    { "name": "eth3" },
    { "name": "wan0" },
    { "name": "erouter0" }
  ]'

addresses=()
addresses+=("    { \"interface\": \"brlan0\", \"prefix\": \"${BRLAN0_IPV4}\" }")
addresses+=("    { \"interface\": \"brlan0\", \"prefix\": \"${BRLAN0_IPV6}\" }")
if [[ -n "${WAN0_IPV4:-}" ]]; then
    addresses+=("    { \"interface\": \"wan0\", \"prefix\": \"${WAN0_IPV4}\" }")
fi
if [[ -n "${WAN0_IPV6:-}" ]]; then
    addresses+=("    { \"interface\": \"wan0\", \"prefix\": \"${WAN0_IPV6}\" }")
fi
addresses+=("    { \"interface\": \"${erouter_iface}\", \"prefix\": \"${EROUTER0_IPV4}\" }")
addresses+=("    { \"interface\": \"${erouter_iface}\", \"prefix\": \"${EROUTER0_IPV6}\" }")

addresses_json=$(printf '%s,\n' "${addresses[@]}")
# $(...) strips trailing newlines, so the array ends in just "," — drop that comma.
addresses_json=${addresses_json%,}

routes_json=$(cat <<JSON
    { "dest": "0.0.0.0/0", "via": "${EROUTER0_IPV4_GATEWAY}", "dev": "${erouter_iface}" },
    { "dest": "::/0", "via": "${EROUTER0_IPV6_GATEWAY}", "dev": "${erouter_iface}" }
JSON
)

cat <<JSON
{
  "version": "1",
  "controllers": {
    "interfaces": {
      "managed": ${managed_json},
      "bridges": {
        "brlan0": {
          "stp": false,
          "forward_delay_secs": null,
          "ageing_time_secs": null,
          "multicast_snooping": null
        }
      },
      "vlans": ${vlans_json},
      "members": [
        { "bridge": "brlan0", "interface": "eth0" },
        { "bridge": "brlan0", "interface": "eth1" },
        { "bridge": "brlan0", "interface": "eth2" },
        { "bridge": "brlan0", "interface": "eth3" }
      ]
    },
    "network": {
      "ipv4_forwarding": true,
      "ipv6_forwarding": true,
      "addresses": [
${addresses_json}
      ],
      "routes": [
${routes_json}
      ]
    }
  }
}
JSON
