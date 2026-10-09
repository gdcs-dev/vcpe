#!/usr/bin/env bash
set -euo pipefail

lookup_peer_ip() {
    local hostname=$1
    local upstream ip
    upstream=$(awk '$1 == "nameserver" { print $2; exit }' /etc/dnsmasq.upstream-resolv.conf)
    [[ -n "$upstream" ]] || return 1
    while IFS= read -r ip; do
        if [[ "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
            printf '%s' "$ip"
            return 0
        fi
    done < <(dig +short +time=1 +tries=1 @"$upstream" "$hostname" A)
    return 1
}

reload_dnsmasq() {
    pkill -HUP dnsmasq 2>/dev/null || true
}

refresh_management_hosts() {
    local seed_file=$1 hosts_file=$2 hostname aliases ip records='' temporary
    while read -r _ hostname aliases; do
        [[ -n "$hostname" ]] || continue
        ip=$(lookup_peer_ip "$hostname" || true)
        if [[ -z "$ip" ]]; then
            ip=$(awk -v name="$hostname" '$2 == name { print $1; exit }' "$hosts_file")
        fi
        [[ -n "$ip" ]] && records+="$ip $hostname${aliases:+ $aliases}"$'\n'
    done < "$seed_file"

    temporary=$(mktemp "${hosts_file}.XXXXXX")
    printf '%s' "$records" > "$temporary"
    chmod 644 "$temporary"
    if cmp -s "$temporary" "$hosts_file"; then
        rm "$temporary"
    else
        mv "$temporary" "$hosts_file"
        reload_dnsmasq
    fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    if [[ "${1:-}" == '--once' ]]; then
        refresh_management_hosts /etc/dnsmasq.hosts /etc/dnsmasq.management.hosts
        exit 0
    fi
    while true; do
        refresh_management_hosts /etc/dnsmasq.hosts /etc/dnsmasq.management.hosts
        sleep 5
    done
fi