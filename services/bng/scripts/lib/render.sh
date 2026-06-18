#!/bin/bash

RENDER_LIB_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

render_customer() {
    local customer_id=$1

    require_customer_id "$customer_id"
    ensure_runtime_root
    python3 "$RENDER_LIB_DIR/customer_config.py" render "$BNG_ROOT" "$customer_id" "$RUNTIME_ROOT"
}