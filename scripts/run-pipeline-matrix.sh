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

for batch in 1 10 50 100; do
  for op in set get; do
    go run ./cmd/bench \
      -client go-redis -op "$op" -pipeline "$batch" \
      -n "$ops" -c "$concurrency" -value-size "$value_size" -warmup "$warmup" \
      | tee "results/pipeline-${batch}-${op}-${timestamp}.jsonl"
  done
done
