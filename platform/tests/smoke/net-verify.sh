#!/bin/bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)

if [[ ! -x "$REPO_ROOT/controlplane/bin/vcpe" ]]; then
	(
		cd "$REPO_ROOT/controlplane"
		go build -o bin/vcpe ./cmd/vcpe
	)
fi

"$REPO_ROOT/controlplane/bin/vcpe" status