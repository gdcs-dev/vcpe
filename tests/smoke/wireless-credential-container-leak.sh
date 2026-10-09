#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
images=()
[[ -n "${VCPE_GATEWAY_IMAGE:-}" ]] && images+=("$VCPE_GATEWAY_IMAGE")
[[ -n "${VCPE_WIRELESS_CLIENT_IMAGE:-}" ]] && images+=("$VCPE_WIRELESS_CLIENT_IMAGE")
if [[ ${#images[@]} -eq 0 ]]; then
    echo 'wireless credential container leak: skipped (set VCPE_GATEWAY_IMAGE and/or VCPE_WIRELESS_CLIENT_IMAGE)'
    exit 0
fi

runtime_dir=$(mktemp -d "$repo_root/.wireless-credential-leak.XXXXXX")
containers=()
cleanup() {
    if [[ ${#containers[@]} -gt 0 ]]; then
        podman rm -f "${containers[@]}" >/dev/null 2>&1 || true
    fi
    rm -rf "$runtime_dir"
}
trap cleanup EXIT

sentinel='VCPE-SENTINEL-container-environment-leak'
credential="$runtime_dir/passphrase"
printf '%s' "$sentinel" >"$credential"
chmod 600 "$credential"

for index in "${!images[@]}"; do
    image=${images[$index]}
    name="vcpe-credential-leak-$index-$$"
    containers+=("$name")
    podman run -d --name "$name" \
        --entrypoint /usr/bin/tail \
        -e RADIO_WIFI_SECURITY=wpa3-personal \
        -e RADIO_WIFI_PASSPHRASE_FILE=/run/vcpe/credentials/home/passphrase \
        -v "$credential:/run/vcpe/credentials/home/passphrase:ro" \
        "$image" -f /dev/null >/dev/null

    for surface in inspect environment logs; do
        case "$surface" in
            inspect) output=$(podman inspect "$name") ;;
            environment) output=$(podman exec "$name" env) ;;
            logs) output=$(podman logs "$name") ;;
        esac
        if [[ "$output" == *"$sentinel"* ]]; then
            echo "wireless credential container leak: sentinel exposed through $surface for $image" >&2
            exit 1
        fi
    done
    podman rm -f "$name" >/dev/null
    containers=("${containers[@]:1}")
done

echo 'wireless credential container leak: ok'
