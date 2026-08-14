#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_DEPLOYED_DIAGNOSTIC:-}" != "1" ]]; then
    echo "SKIP: set VCPE_RUN_DEPLOYED_DIAGNOSTIC=1 to provision the full diagnostic stack"
    exit 0
fi

if ! command -v podman >/dev/null 2>&1; then
    echo "SKIP: podman is not installed"
    exit 0
fi

deployment=example-full
manifest="$repo_root/manifests/dev/example-full.yaml"
vcpe="$repo_root/controlplane/bin/vcpe"
state_root=$(mktemp -d)
shadow_path=$(mktemp -d)

cleanup() {
    "$vcpe" down --name "$deployment" --state-root "$state_root" >/dev/null 2>&1 || true
    rm -rf "$state_root" "$shadow_path"
}
trap cleanup EXIT

cd "$repo_root"
make build
"$vcpe" up --manifest "$manifest" --state-root "$state_root"

cat >"$shadow_path/podman" <<'EOF'
#!/bin/sh
echo "diagnose unexpectedly invoked podman" >&2
exit 99
EOF
chmod +x "$shadow_path/podman"

diagnostic_output=$(PATH="$shadow_path:$PATH" "$vcpe" diagnose \
    --name "$deployment" --from gateway --to webpa --client-service apparmor-simulator \
    --state-root "$state_root" --json 2>&1)
diagnostic_status=$?

if [[ $diagnostic_status -ne 0 ]]; then
    echo "expected healthy diagnostic exit status" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi
if [[ "$diagnostic_output" == *"unexpectedly invoked podman"* ]]; then
    echo "$diagnostic_output" >&2
    exit 1
fi
for expected in \
    '"schemaVersion": "vcpe.dev/diagnostic/v1"' \
    '"edgeId": "talaria-dns"' \
    '"edgeId": "device-registration"' \
    '"key": "client-evidence"' \
    '"value": "online"' \
    '"state": "passed"'; do
    if [[ "$diagnostic_output" != *"$expected"* ]]; then
        echo "missing diagnostic output: $expected" >&2
        echo "$diagnostic_output" >&2
        exit 1
    fi
done

podman stop "${deployment}-webpa-1" >/dev/null

set +e
failure_output=$(PATH="$shadow_path:$PATH" "$vcpe" diagnose \
    --name "$deployment" --from gateway --to webpa --client-service apparmor-simulator \
    --state-root "$state_root" --json 2>&1)
failure_status=$?
set -e

if [[ $failure_status -eq 0 ]]; then
    echo "expected diagnostic failure after stopping WebPA" >&2
    exit 1
fi
if [[ "$failure_output" == *"unexpectedly invoked podman"* ]]; then
    echo "$failure_output" >&2
    exit 1
fi
if [[ "$failure_output" != *'"firstFailure": "talaria-dns"'* && \
      "$failure_output" != *'"firstFailure": "talaria-transport"'* ]]; then
    echo "expected DNS or transport as first failure after stopping WebPA" >&2
    echo "$failure_output" >&2
    exit 1
fi
if [[ "$failure_output" != *'"state": "skipped"'* ]]; then
    echo "expected downstream skipped stages after the WebPA outage" >&2
    echo "$failure_output" >&2
    exit 1
fi

echo "PASS: deployed healthy and failed diagnostics used loopback HTTP and returned causal graphs"