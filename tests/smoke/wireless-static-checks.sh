#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"

unformatted=$(find \
    controlplane/internal/app \
    controlplane/internal/backend/podman \
    controlplane/internal/compose \
    controlplane/internal/hwsim \
    controlplane/internal/manifest \
    controlplane/internal/persist \
    controlplane/internal/plan \
    controlplane/internal/planner \
    controlplane/internal/render \
    controlplane/internal/runtimeinit \
    controlplane/internal/secrets \
    controlplane/internal/state \
    controlplane/internal/types/gateway \
    controlplane/internal/types/genericcontainer \
    -name '*.go' -type f -print0 | xargs -0 gofmt -l)
if [[ -n "$unformatted" ]]; then
    echo "gofmt required:" >&2
    echo "$unformatted" >&2
    exit 1
fi

if ! command -v podman-compose >/dev/null 2>&1; then
    echo 'podman-compose is required to validate generated wireless Compose files' >&2
    exit 1
fi

(
    cd controlplane
    go vet ./internal/hwsim ./internal/backend/podman ./internal/compose ./internal/manifest ./internal/planner ./internal/persist ./internal/render ./internal/runtimeinit/... ./internal/secrets ./internal/state ./internal/types/gateway ./internal/types/genericcontainer ./internal/app
    go test ./internal/compose ./internal/manifest ./internal/plan ./internal/planner ./internal/persist ./internal/render ./internal/runtimeinit/contract ./internal/secrets ./internal/state ./internal/types/gateway ./internal/types/genericcontainer ./internal/app
)

bash -n \
    services/gateway/container/entrypoint.sh \
    services/gateway/container/hostapd-wrapper.sh \
    services/wireless-client/bootstrap.sh \
    tests/smoke/controlplane-wireless-fake-manager.sh \
    tests/smoke/controlplane-mesh-roaming-machine.sh \
    tests/smoke/gateway-wireless-entrypoint.sh \
    tests/smoke/gateway-hostapd-wrapper.sh \
    tests/smoke/gateway-vap-container.sh \
    tests/smoke/gateway-wireless-image-content.sh \
    tests/smoke/wireless-client-bootstrap.sh \
    tests/smoke/wireless-credential-container-leak.sh \
    tests/smoke/wireless-security-image-capabilities.sh

bash tests/smoke/gateway-wireless-image-content.sh
bash tests/smoke/gateway-hostapd-wrapper.sh
bash tests/smoke/wireless-client-bootstrap.sh
bash tests/smoke/controlplane-wireless-fake-manager.sh
bash tests/smoke/wireless-credential-container-leak.sh
bash tests/smoke/wireless-security-image-capabilities.sh
openspec validate tri-band-multi-vap-wireless --strict
openspec validate wireless-mesh-backhaul --strict
openspec validate wireless-rf-roaming-scenarios --strict
git diff --check

echo 'wireless static checks: ok'