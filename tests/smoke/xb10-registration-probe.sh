#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
probe="$repo_root/services/xb10/container/health-probe.sh"
mock_dir=$(mktemp -d)
trap 'rm -rf "$mock_dir"' EXIT

cat >"$mock_dir/ip" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$mock_dir/systemctl" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$mock_dir/curl" <<'EOF'
#!/bin/sh
printf '%s\n' "${VCPE_TALARIA_RESPONSE:-{\"devices\":[{\"id\":\"mac:aabbccddeeff\"}]}}"
EOF
cat >"$mock_dir/jq" <<'EOF'
#!/bin/sh
input=$(cat)
case "$input" in
  *"\"id\":\"${4}\""*) exit 0 ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$mock_dir/ip" "$mock_dir/systemctl" "$mock_dir/curl" "$mock_dir/jq"

PATH="$mock_dir:$PATH" VCPE_HEALTH_SERIAL=aabbccddeeff sh "$probe" webpa-registration
if PATH="$mock_dir:$PATH" VCPE_HEALTH_SERIAL=aabbccddeeff \
  VCPE_TALARIA_RESPONSE='{"devices":[]}' sh "$probe" webpa-registration; then
    echo "expected probe failure after the Talaria registration expires" >&2
    exit 1
fi
