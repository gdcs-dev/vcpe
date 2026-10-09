#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
containerfile="$root/Containerfile"

grep --quiet 'git init /src' "$containerfile"
grep --quiet 'db_file = "/app/data/webconfig.sqlite"' "$root/container/webconfig.conf"
grep --quiet 'rm -f /app/data/webconfig.sqlite' "$root/container/entrypoint.sh"
if grep -Eq '/src/.*(sqlite|data)|/src.*(sqlite|data)' "$root/container/entrypoint.sh" "$root/container/webconfig.conf"; then
    echo "WebConfig runtime state must not be placed in the upstream source checkout" >&2
    exit 1
fi