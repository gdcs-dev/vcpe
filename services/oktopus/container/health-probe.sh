#!/bin/sh
set -eu

for unit in mongod nats ws ws-adapter controller taas frontend nginx; do
    systemctl is-active --quiet "$unit.service"
done