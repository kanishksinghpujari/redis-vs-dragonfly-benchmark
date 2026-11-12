#!/usr/bin/env bash
set -euo pipefail

mkdir -p results
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
ops="${OPS:-100000}"
warmup="${WARMUP:-10000}"
value_size="${VALUE_SIZE:-128}"
concurrency="${CONCURRENCY:-32}"

docker compose up -d --wait
trap 'docker compose down' EXIT

# Both clients use pipeline=1. Rueidis auto-pipelining is disabled in the
# benchmark client, so this measures each library's direct-command path.
for client in go-redis rueidis; do
  for op in set get; do
    go run ./cmd/bench \
      -client "$client" -op "$op" -pipeline 1 \
      -n "$ops" -c "$concurrency" -value-size "$value_size" -warmup "$warmup" \
      | tee "results/client-${client}-${op}-${timestamp}.jsonl"
  done
done
