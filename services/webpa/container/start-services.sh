#!/bin/bash
set -euo pipefail

declare -a SERVICE_NAMES=()
declare -a SERVICE_PIDS=()

start_service() {
	local service_name=$1
	shift
	local service_bin

	service_bin=$(command -v "$service_name" 2>/dev/null || echo "/usr/bin/$service_name")
	if (( $# > 0 )); then
		"$service_bin" "$@" &
	else
		"$service_bin" &
	fi
	SERVICE_NAMES+=("$service_name")
	SERVICE_PIDS+=("$!")
}

wait_for_http() {
	local url=$1
	local attempts=0

	until curl -sf "$url" >/dev/null 2>&1; do
		attempts=$((attempts + 1))
		if (( attempts >= 40 )); then
			echo "timed out waiting for $url" >&2
			exit 1
		fi
		sleep 0.25
	done
}

stop_services() {
	if (( ${#SERVICE_PIDS[@]} > 0 )); then
		kill "${SERVICE_PIDS[@]}" 2>/dev/null || true
	fi
	exit 0
}

start_service consul agent -config-dir=/etc/consul.d/
wait_for_http http://127.0.0.1:8500/v1/status/leader

start_service talaria
start_service scytale
start_service tr1d1um
start_service argus
start_service caduceus
start_service themis

trap stop_services TERM INT

status_parts=()
for index in "${!SERVICE_NAMES[@]}"; do
	status_parts+=("${SERVICE_NAMES[$index]}=${SERVICE_PIDS[$index]}")
done
echo "webpa services started (${status_parts[*]})"

wait "${SERVICE_PIDS[@]}"
