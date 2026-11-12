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

for op in set get; do
  go run ./cmd/bench \
    -op "$op" \
    -n "$ops" \
    -c "$concurrency" \
    -value-size "$value_size" \
    -warmup "$warmup" \
    | tee "results/${op}-${timestamp}.jsonl"
done
