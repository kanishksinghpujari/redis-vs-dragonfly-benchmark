#!/usr/bin/env bash
set -euo pipefail

mkdir -p results
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
ops="${OPS:-50000}"
warmup="${WARMUP:-5000}"
value_size="${VALUE_SIZE:-128}"
concurrency="${CONCURRENCY:-32}"

docker compose up -d --wait
trap 'docker compose down' EXIT

wait_for_proxy() {
  for _ in $(seq 1 30); do
    if curl -fsS http://localhost:8474/version >/dev/null; then return 0; fi
    sleep 1
  done
  echo "Toxiproxy API did not become ready" >&2
  exit 1
}

configure_proxy() {
  local name="$1" listen="$2" upstream="$3" delay_ms="$4"
  curl -fsS -X DELETE "http://localhost:8474/proxies/$name" >/dev/null 2>&1 || true
  curl -fsS -X POST http://localhost:8474/proxies \
    -H 'Content-Type: application/json' \
    -d "{"name":"$name","listen":"0.0.0.0:$listen","upstream":"$upstream","enabled":true}" >/dev/null
  if [ "$delay_ms" -gt 0 ]; then
    curl -fsS -X POST "http://localhost:8474/proxies/$name/toxics" \
      -H 'Content-Type: application/json' \
      -d "{"name":"response-latency","type":"latency","stream":"downstream","toxicity":1.0,"attributes":{"latency":$delay_ms,"jitter":0}}" >/dev/null
  fi
}

wait_for_proxy
for delay in 0 1 5; do
  configure_proxy redis-latency 26379 redis:6379 "$delay"
  configure_proxy dragonfly-latency 26380 dragonfly:6379 "$delay"
  for op in set get; do
    go run ./cmd/bench \
      -client go-redis -op "$op" -pipeline 1 \
      -redis-addr localhost:26379 -dragonfly-addr localhost:26380 \
      -n "$ops" -c "$concurrency" -value-size "$value_size" -warmup "$warmup" \
      | tee "results/network-${delay}ms-${op}-${timestamp}.jsonl"
  done
done
