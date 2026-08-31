#!/bin/bash
set -euo pipefail

event=${1:-}
lease_ip=${2:-}
client_mac=${3:-}
client_name=${4:-}
map_file=${DHCPD_NOTIFY_MAP_FILE:-/etc/dnsmasq.dhcp-hosts.map}
state_file=${DHCPD_NOTIFY_STATE_FILE:-/var/lib/dhcp/dnsmasq-dhcp.state}
hosts_file=${DHCPD_NOTIFY_HOSTS_FILE:-/etc/dnsmasq.dhcp.hosts}
log_file=${DHCPD_NOTIFY_LOG_FILE:-/var/log/dhcpd-notify.log}
lock_dir=${DHCPD_NOTIFY_LOCK_DIR:-/run/dhcpd-notify.lock}

normalize_mac() {
	local raw
	local octets=()
	local octet value normalized=""
	raw=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
	IFS=: read -r -a octets <<<"$raw"
	[[ ${#octets[@]} -eq 6 ]] || return 1
	for octet in "${octets[@]}"; do
		[[ "$octet" =~ ^[0-9a-f]{1,2}$ ]] || return 1
		value=$((16#$octet))
		printf -v octet '%02x' "$value"
		normalized+="${normalized:+:}$octet"
	done
	printf '%s\n' "$normalized"
}

lookup_mapped_aliases() {
	local normalized_mac=$1

	[[ -f "$map_file" ]] || return 1
	awk -v mac="$normalized_mac" '
		tolower($1) == mac {
			$1 = ""
			sub(/^[[:space:]]+/, "")
			print
			exit
		}
	' "$map_file"
}

acquire_lock() {
	local attempt
	for attempt in $(seq 1 200); do
		if mkdir "$lock_dir" 2>/dev/null; then
			trap 'rmdir "$lock_dir" 2>/dev/null || true' EXIT
			return 0
		fi
		sleep 0.01
	done
	echo "timed out acquiring DHCP DNS state lock $lock_dir" >&2
	return 1
}

refresh_hosts_file() {
	local temp_hosts

	temp_hosts=$(mktemp "${hosts_file}.tmp.XXXXXX")
	if [[ -f "$state_file" ]]; then
		awk 'NF >= 3 {
			ip = $2
			$1 = ""
			$2 = ""
			sub(/^[[:space:]]+/, "")
			print ip " " $0
		}' "$state_file" >"$temp_hosts"
	fi
	chmod 0644 "$temp_hosts"
	mv "$temp_hosts" "$hosts_file"
	if [[ "${DHCPD_NOTIFY_SKIP_RELOAD:-}" != "1" ]]; then
		pkill -HUP dnsmasq 2>/dev/null || true
	fi
}

commit_state_record() {
	local mac=$1
	local ip=$2
	local aliases=$3
	local temp_state

	temp_state=$(mktemp "${state_file}.tmp.XXXXXX")
	if [[ -f "$state_file" ]]; then
		awk -v mac="$mac" 'tolower($1) != mac { print }' "$state_file" >"$temp_state"
	fi
	printf '%s %s %s\n' "$mac" "$ip" "$aliases" >>"$temp_state"
	chmod 0644 "$temp_state"
	mv "$temp_state" "$state_file"
}

remove_state_record() {
	local mac=$1
	local ip=$2
	local temp_state

	temp_state=$(mktemp "${state_file}.tmp.XXXXXX")
	if [[ -f "$state_file" ]]; then
		awk -v mac="$mac" -v ip="$ip" '
			!(tolower($1) == mac && $2 == ip) { print }
		' "$state_file" >"$temp_state"
	fi
	chmod 0644 "$temp_state"
	mv "$temp_state" "$state_file"
}

mkdir -p "$(dirname "$state_file")" "$(dirname "$hosts_file")" "$(dirname "$log_file")" "$(dirname "$lock_dir")"
touch "$state_file" "$hosts_file" "$log_file"

normalized_mac=$(normalize_mac "$client_mac" || true)
aliases=$(lookup_mapped_aliases "$normalized_mac" || true)

printf '%s event=%s ip=%s mac=%s client_host=%s aliases=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$event" "$lease_ip" "$normalized_mac" "$client_name" "$aliases" >>"$log_file"

[[ -n "$normalized_mac" && -n "$aliases" ]] || exit 0
acquire_lock

case "$event" in
	commit)
		commit_state_record "$normalized_mac" "$lease_ip" "$aliases"
		refresh_hosts_file
		;;
	release|expiry)
		remove_state_record "$normalized_mac" "$lease_ip"
		refresh_hosts_file
		;;
	*)
		exit 0
		;;
esac
