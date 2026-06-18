#!/bin/bash
# Check all configured xmidt health endpoints.
curl -sf http://127.0.0.1:6201/health >/dev/null 2>&1 || exit 1   # talaria
curl -sf http://127.0.0.1:6301/health >/dev/null 2>&1 || exit 1   # scytale
curl -sf http://127.0.0.1:6102/health >/dev/null 2>&1 || exit 1   # tr1d1um
curl -sf http://127.0.0.1:6602/health >/dev/null 2>&1 || exit 1   # argus
curl -sf http://127.0.0.1:6001/health >/dev/null 2>&1 || exit 1   # caduceus
curl -sf http://127.0.0.1:6401/health >/dev/null 2>&1 || exit 1   # petasos
curl -sf http://127.0.0.1:6504/health >/dev/null 2>&1 || exit 1   # themis
exit 0
