#!/usr/bin/env bash
set -euo pipefail

response=$(mktemp)
trap 'rm -f "$response"' EXIT

content_type=$(curl --fail --silent --show-error --dump-header - --output "$response" \
    http://127.0.0.1:9000/api/v1/device/02:00:00:00:00:01/config \
    | awk 'BEGIN { IGNORECASE = 1 } /^Content-Type:/ { print $2 }' \
    | tr -d '\r')

[[ "$content_type" == multipart/mixed* ]]
grep --quiet 'privatessid' "$response"