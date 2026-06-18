#!/bin/bash

NAT_SOURCE=10.10.10.0/24
NAT_EXCLUDE_DEV=wan
NAT_EXCLUDE_DEST=10.10.10.0/24

nat_rule_args() {
    printf '%s\n' "-s $NAT_SOURCE ! -o $NAT_EXCLUDE_DEV ! -d $NAT_EXCLUDE_DEST -j MASQUERADE"
}

ensure_nat_rule() {
    local rule
    rule=$(nat_rule_args)
    if ! run_linux_host_root iptables -t nat -C POSTROUTING $rule 2>/dev/null; then
        run_linux_host_root iptables -t nat -A POSTROUTING $rule
    fi
}

verify_nat_rule() {
    local rule
    rule=$(nat_rule_args)
    run_linux_host_root iptables -t nat -C POSTROUTING $rule >/dev/null 2>&1 || die "missing NAT rule"
}

cleanup_nat_rule() {
    local rule
    rule=$(nat_rule_args)
    if run_linux_host_root iptables -t nat -C POSTROUTING $rule >/dev/null 2>&1; then
        run_linux_host_root iptables -t nat -D POSTROUTING $rule
    fi
}