#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
probe="$repo_root/services/bng/container/health-probe.sh"
mock_dir=$(mktemp -d)
trap 'rm -rf "$mock_dir"' EXIT

cat >"$mock_dir/pgrep" <<'EOF'
#!/bin/sh
test "${VCPE_MISSING_PROCESS:-}" != "$2"
EOF
cat >"$mock_dir/ss" <<'EOF'
#!/bin/sh
printf '%s\n' 'LISTEN 0 511 *:80 *:*'
printf '%s\n' 'LISTEN 0 511 *:1883 *:*'
EOF
chmod +x "$mock_dir/pgrep" "$mock_dir/ss"

PATH="$mock_dir:$PATH" sh "$probe"
if VCPE_MISSING_PROCESS=mosquitto PATH="$mock_dir:$PATH" sh "$probe"; then
    echo "expected probe failure when mosquitto is missing" >&2
    exit 1
fi