#!/bin/sh
# Renders the runtime LiveKit config from deploy/livekit/livekit.yaml plus the
# environment, then starts the server. Without keys the container idles
# instead of crash-looping, so a stack without video still deploys.
set -eu

if [ -z "${LIVEKIT_API_KEY:-}" ] || [ -z "${LIVEKIT_API_SECRET:-}" ]; then
  echo "livekit: LIVEKIT_API_KEY / LIVEKIT_API_SECRET not set; video rooms disabled (docs/video-rooms.md)"
  exec sleep 2147483647
fi

# LiveKit refuses secrets shorter than 32 characters.
if [ "${#LIVEKIT_API_SECRET}" -lt 32 ]; then
  echo "livekit: LIVEKIT_API_SECRET must be at least 32 characters" >&2
  exit 1
fi

export LIVEKIT_KEYS="${LIVEKIT_API_KEY}: ${LIVEKIT_API_SECRET}"
conf=/tmp/livekit.yaml
cp /etc/livekit/livekit.yaml "$conf"

cat >>"$conf" <<YAML

rtc:
  tcp_port: ${LIVEKIT_RTC_TCP_PORT:-7881}
  port_range_start: ${LIVEKIT_RTC_PORT_START:-50000}
  port_range_end: ${LIVEKIT_RTC_PORT_END:-50100}
  use_external_ip: ${LIVEKIT_USE_EXTERNAL_IP:-true}
YAML

if [ -n "${LIVEKIT_WEBHOOK_URL:-}" ]; then
  cat >>"$conf" <<YAML

webhook:
  api_key: "${LIVEKIT_API_KEY}"
  urls:
    - "${LIVEKIT_WEBHOOK_URL}"
YAML
fi

# Embedded TURN: UDP 3478 for restrictive NATs and TURN/TLS on 5349 for
# networks that only allow TLS. external_tls: the reverse proxy (Traefik TCP
# router, HostSNI = LIVEKIT_TURN_DOMAIN) terminates TLS with its certificate.
if [ -n "${LIVEKIT_TURN_DOMAIN:-}" ]; then
  cat >>"$conf" <<YAML

turn:
  enabled: true
  domain: "${LIVEKIT_TURN_DOMAIN}"
  udp_port: ${LIVEKIT_TURN_UDP_PORT:-3478}
  tls_port: ${LIVEKIT_TURN_TLS_PORT:-5349}
  external_tls: true
  relay_range_start: ${LIVEKIT_TURN_RELAY_START:-50200}
  relay_range_end: ${LIVEKIT_TURN_RELAY_END:-50300}
YAML
fi

exec /livekit-server --config "$conf" "$@"
