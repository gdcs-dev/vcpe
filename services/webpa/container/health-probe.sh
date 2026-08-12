#!/bin/sh
set -eu

case ${1:-} in
talaria) endpoint=http://127.0.0.1:6201/health ;;
scytale) endpoint=http://127.0.0.1:6301/health ;;
tr1d1um) endpoint=http://127.0.0.1:6102/health ;;
argus) endpoint=http://127.0.0.1:6602/health ;;
caduceus) endpoint=http://127.0.0.1:6001/health ;;
themis) endpoint=http://127.0.0.1:6504/health ;;
*)
    echo "usage: $0 {talaria|scytale|tr1d1um|argus|caduceus|themis}" >&2
    exit 2
    ;;
esac

curl -fsS "$endpoint" >/dev/null