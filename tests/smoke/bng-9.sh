#!/bin/bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

exec "$REPO_ROOT/services/bng/tests/smoke/bng-9.sh" "$@"