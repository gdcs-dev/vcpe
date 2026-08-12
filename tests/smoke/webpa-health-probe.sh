#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
probe="$repo_root/services/webpa/container/health-probe.sh"
mock_dir=$(mktemp -d)
trap 'rm -rf "$mock_dir"' EXIT

cat >"$mock_dir/curl" <<'EOF'
#!/bin/sh
test "${VCPE_FAIL_ENDPOINT:-}" != "$2"
EOF
chmod +x "$mock_dir/curl"

for check in talaria scytale tr1d1um argus caduceus themis; do
    PATH="$mock_dir:$PATH" sh "$probe" "$check"
done
if VCPE_FAIL_ENDPOINT=http://127.0.0.1:6602/health PATH="$mock_dir:$PATH" sh "$probe" argus; then
    echo "expected probe failure when an XMiDT endpoint is unavailable" >&2
    exit 1
fi