#!/bin/bash
set -euo pipefail

ip addr show brlan0 >/dev/null
ip addr show wan0 >/dev/null
ip addr show erouter0 >/dev/null