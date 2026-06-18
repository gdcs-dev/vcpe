#!/usr/bin/env python3
# pyright: reportMissingImports=false

import re
import shutil
import sys
from ipaddress import ip_interface
from pathlib import Path

try:
    import yaml  # pyright: ignore[reportMissingImports]
except ImportError as exc:
    raise SystemExit("missing required python module: pyyaml") from exc


def die(message: str) -> None:
    raise SystemExit(message)


def load_customer(repo_root: Path, customer_id: str) -> dict:
    customer_file = repo_root / "customers" / f"{customer_id}.yaml"
    if not customer_file.exists():
        die(f"missing customer config: {customer_file}")
    with customer_file.open("r", encoding="utf-8") as handle:
        data = yaml.safe_load(handle)
    if not isinstance(data, dict):
        die(f"invalid customer config: {customer_file}")
    return data


def attachments_by_network(customer: dict) -> dict:
    attachments = customer.get("attachments", [])
    return {attachment["network"]: attachment for attachment in attachments}


def access_entries(customer: dict) -> list:
    return customer.get("access", [])


def load_env_file(path: Path) -> dict[str, str]:
    data: dict[str, str] = {}
    if not path.exists():
        return data

    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        data[key.strip()] = value.strip()
    return data


def copy_inputs(repo_root: Path, customer_root: Path) -> None:
    (customer_root / "etc" / "dhcp").mkdir(parents=True, exist_ok=True)
    (customer_root / "var" / "www" / "html").mkdir(parents=True, exist_ok=True)

    shutil.copy2(repo_root / "templates" / "dhcpd.conf.tpl", customer_root / "etc" / "dhcp" / "dhcpd.conf")
    shutil.copy2(repo_root / "templates" / "dhcpd6.conf.tpl", customer_root / "etc" / "dhcp" / "dhcpd6.conf")
    shutil.copy2(repo_root / "templates" / "ntp.conf.tpl", customer_root / "etc" / "ntp.conf")
    shutil.copy2(repo_root / "assets" / "DCMresponse.txt", customer_root / "var" / "www" / "html" / "DCMresponse.txt")


def render_dnsmasq(customer_root: Path, customer: dict) -> None:
    management = customer["management"]
    listen_ipv4 = ["127.0.0.1", management["ipv4"].split("/", 1)[0]]
    webpa_names = [
        "webpa",
        "consul",
        "talaria",
        "scytale",
        "tr1d1um",
        "argus",
        "caduceus",
        "petasos",
        "themis",
    ]

    for entry in access_entries(customer):
        listen_ipv4.append(entry["ipv4"].split("/", 1)[0])

    lines = [
        "port=53",
        "bind-dynamic",
        f"listen-address={','.join(listen_ipv4)}",
        "no-hosts",
        "addn-hosts=/etc/dnsmasq.hosts",
        "addn-hosts=/etc/dnsmasq.dynamic.hosts",
        "resolv-file=/etc/dnsmasq.upstream-resolv.conf",
        "cache-size=0",
    ]
    (customer_root / "etc" / "dnsmasq.conf").write_text("\n".join(lines) + "\n", encoding="utf-8")

    hosts_lines = [
        f"10.10.10.210 {' '.join(webpa_names)}",
        f"2001:dbf:0:1::210 {' '.join(webpa_names)}",
    ]
    (customer_root / "etc" / "dnsmasq.hosts").write_text("\n".join(hosts_lines) + "\n", encoding="utf-8")


def render_dnsmasq_dynamic_files(repo_root: Path, customer_root: Path, customer: dict) -> None:
    customer_id = str(customer["customer_id"])
    mv1_env = load_env_file(repo_root.parent / "mv1" / "customers" / f"{customer_id}.env")
    lines: list[str] = []
    subnet_lines: list[str] = []

    erouter_mac = mv1_env.get("EROUTER0_MAC")
    if erouter_mac:
        lines.append(f"{erouter_mac.lower()} xb10-{customer_id}")

    wan_mac = mv1_env.get("WAN0_MAC")
    if wan_mac:
        lines.append(f"{wan_mac.lower()} xb10-cm-{customer_id}")

    for entry in access_entries(customer):
        hostname = None
        if entry["network"] == "wan":
            hostname = f"xb10-{customer_id}"
        elif entry["network"] == "cm":
            hostname = f"xb10-cm-{customer_id}"

        if hostname is None:
            continue

        subnet_lines.append(f"{ip_interface(entry['ipv4']).network} {hostname}")

    (customer_root / "etc" / "dnsmasq.dhcp-hosts.map").write_text("\n".join(lines) + ("\n" if lines else ""), encoding="utf-8")
    (customer_root / "etc" / "dnsmasq.dhcp-subnets.map").write_text("\n".join(subnet_lines) + ("\n" if subnet_lines else ""), encoding="utf-8")
    (customer_root / "etc" / "dnsmasq.dynamic.hosts").write_text("", encoding="utf-8")


def render_ports_conf(customer_root: Path, customer: dict) -> None:
    first_access = access_entries(customer)[0]
    ipv4 = first_access["ipv4"].split("/", 1)[0]
    ipv6 = first_access["ipv6"].split("/", 1)[0]
    content = f"""# If you just change the port or add more ports here, you will likely also
# have to change the VirtualHost statement in
# /etc/apache2/sites-enabled/000-default.conf

Listen {ipv4}:80
Listen [{ipv6}]:80

<IfModule ssl_module>
\tListen 443
</IfModule>

<IfModule mod_gnutls.c>
\tListen 443
</IfModule>

# vim: syntax=apache ts=4 sw=4 sts=4 sr noet
"""
    (customer_root / "etc" / "ports.conf").write_text(content, encoding="utf-8")


def apply_template_replacements(customer_root: Path, customer: dict) -> None:
    for replacement in customer.get("template_replacements", []):
        target = customer_root / replacement["file"]
        content = target.read_text(encoding="utf-8")
        content = re.sub(replacement["pattern"], replacement["replacement"], content)
        target.write_text(content, encoding="utf-8")


def render_network_startup(customer_root: Path, customer: dict) -> None:
    attachments = attachments_by_network(customer)
    access = access_entries(customer)
    management = customer["management"]
    lines = ["#!/bin/bash", "set -euo pipefail"]

    mgmt = attachments["mgmt"]
    lines.extend(
        [
            f"ip link set {mgmt['interface']} up",
            f"ip addr replace {management['ipv4']} dev {mgmt['interface']}",
            f"ip -6 addr replace {management['ipv6']} dev {mgmt['interface']}",
            "ip route replace default via 10.10.10.1",
            "ip -6 route replace default via 2001:dbf:0:1::1",
            "",
        ]
    )

    for network_name in ("wan", "cm"):
        attachment = attachments[network_name]
        interface = attachment["interface"]
        lines.append(f"ip link set {interface} up")
        if any(entry.get("parent") == interface for entry in access):
            lines.append(f"ip addr flush dev {interface} || true")

        direct_entries = [entry for entry in access if entry["interface"] == interface and "vlan" not in entry]
        for entry in direct_entries:
            lines.extend(
                [
                    f"ip addr replace {entry['ipv4']} dev {interface}",
                    f"ip -6 addr replace {entry['ipv6']} dev {interface}",
                ]
            )
        lines.append("")

    for entry in access:
        if "vlan" not in entry:
            continue
        lines.extend(
            [
                f"ip link add link {entry['parent']} name {entry['interface']} type vlan id {entry['vlan']} || true",
                f"ip link set {entry['interface']} up",
                f"ip addr replace {entry['ipv4']} dev {entry['interface']}",
                f"ip -6 addr replace {entry['ipv6']} dev {entry['interface']}",
                "",
            ]
        )

    output = "\n".join(lines).rstrip() + "\n"
    target = customer_root / "etc" / "network-startup.sh"
    target.write_text(output, encoding="utf-8")
    target.chmod(0o755)


def render_radvd_conf(customer_root: Path, customer: dict) -> None:
    blocks = []
    for entry in access_entries(customer):
        radvd = entry["radvd"]
        block = [
            f"interface {entry['interface']}",
            "{",
            f"    AdvSendAdvert {'on' if radvd.get('send_advert', True) else 'off'};",
            f"    AdvManagedFlag {'on' if radvd['managed_flag'] else 'off'};",
            f"    AdvOtherConfigFlag {'on' if radvd['other_config_flag'] else 'off'};",
            f"    AdvDefaultLifetime {radvd['default_lifetime']};",
        ]
        if "min_delay_between_ras" in radvd:
            block.append(f"    MinDelayBetweenRAs {radvd['min_delay_between_ras']};")
        block.extend(
            [
                f"    MinRtrAdvInterval {radvd['min_rtr_adv_interval']};",
                f"    MaxRtrAdvInterval {radvd['max_rtr_adv_interval']};",
                "};",
            ]
        )
        blocks.append("\n".join(block))
    (customer_root / "etc" / "radvd.conf").write_text("\n\n".join(blocks) + "\n", encoding="utf-8")


def render_sysctl_conf(customer_root: Path, customer: dict) -> None:
    lines = ["net.ipv4.ip_forward=1", "net.ipv6.conf.all.forwarding=1"]
    for entry in access_entries(customer):
        lines.append(f"net.ipv6.conf.{entry['interface']}.accept_ra=2")
    (customer_root / "etc" / "sysctl.conf").write_text("\n".join(lines) + "\n", encoding="utf-8")


def render_service_interfaces(customer_root: Path, customer: dict) -> None:
    interfaces = " ".join(entry["interface"] for entry in access_entries(customer))
    content = f'DHCP4_INTERFACES="{interfaces}"\nDHCP6_INTERFACES="{interfaces}"\n'
    (customer_root / "etc" / "service-interfaces.env").write_text(content, encoding="utf-8")


def render_iptables_rules(customer_root: Path, customer: dict) -> None:
    lines = [
        "*nat",
        ":PREROUTING ACCEPT [0:0]",
        ":INPUT ACCEPT [0:0]",
        ":OUTPUT ACCEPT [0:0]",
        ":POSTROUTING ACCEPT [0:0]",
    ]
    for cidr in customer.get("nat_ipv4_cidrs", []):
        lines.append(f"-A POSTROUTING -s {cidr} -o eth0 ! -d 10.10.10.0/24 -j MASQUERADE")
    lines.append("COMMIT")
    (customer_root / "etc" / "iptables.rules.v4").write_text("\n".join(lines) + "\n", encoding="utf-8")
    (customer_root / "etc" / "iptables.rules.v6").write_text("", encoding="utf-8")


def render_customer(repo_root: Path, customer_id: str, runtime_root: Path) -> None:
    customer = load_customer(repo_root, customer_id)
    customer_root = runtime_root / customer_id
    copy_inputs(repo_root, customer_root)
    render_ports_conf(customer_root, customer)
    apply_template_replacements(customer_root, customer)
    render_network_startup(customer_root, customer)
    render_radvd_conf(customer_root, customer)
    render_sysctl_conf(customer_root, customer)
    render_service_interfaces(customer_root, customer)
    render_iptables_rules(customer_root, customer)
    render_dnsmasq(customer_root, customer)
    render_dnsmasq_dynamic_files(repo_root, customer_root, customer)
    print(f"rendered customer {customer_id} into {customer_root}")


def emit_compose_env(repo_root: Path, customer_id: str, image_name: str) -> None:
    customer = load_customer(repo_root, customer_id)
    attachments = attachments_by_network(customer)
    runtime_root = (repo_root / "runtime").resolve()
    print(f"CUSTOMER_ID={customer_id}")
    print(f"IMAGE_NAME={image_name}")
    print(f"RUNTIME_ROOT={runtime_root}")
    print(f"MGMT_MAC={attachments['mgmt']['mac']}")
    print(f"WAN_MAC={attachments['wan']['mac']}")
    print(f"CM_MAC={attachments['cm']['mac']}")
    print(f"WAN_NETWORK=wan-{customer_id}")
    print(f"CM_NETWORK=cm-{customer_id}")


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        die("usage: customer_config.py <render|compose-env> ...")

    command = argv[1]
    if command == "render":
        if len(argv) != 5:
            die("usage: customer_config.py render <repo_root> <customer_id> <runtime_root>")
        repo_root = Path(argv[2]).resolve()
        customer_id = argv[3]
        runtime_root = Path(argv[4]).resolve()
        render_customer(repo_root, customer_id, runtime_root)
        return 0

    if command == "compose-env":
        if len(argv) != 5:
            die("usage: customer_config.py compose-env <repo_root> <customer_id> <image_name>")
        repo_root = Path(argv[2]).resolve()
        customer_id = argv[3]
        image_name = argv[4]
        emit_compose_env(repo_root, customer_id, image_name)
        return 0

    die(f"unsupported command: {command}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))