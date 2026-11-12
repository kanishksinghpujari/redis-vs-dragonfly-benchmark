#!/usr/bin/env bash
set -euo pipefail

mkdir -p results
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
ops="${OPS:-30000}"
warmup="${WARMUP:-3000}"
trials="${TRIALS:-3}"
client="${CLIENT:-go-redis}"

docker compose up -d --wait
stats_file="results/container-stats-${timestamp}.csv"
echo "timestamp,container,cpu_percent,memory_usage" > "$stats_file"
(
  while true; do
    now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    docker stats --no-stream --format "{{.Name}},{{.CPUPerc}},{{.MemUsage}}" 2>/dev/null \
      | while IFS= read -r line; do echo "$now,$line"; done >> "$stats_file"
    sleep 1
  done
) &
stats_pid=$!
cleanup() {
  kill "$stats_pid" 2>/dev/null || true
  docker compose down
}
trap cleanup EXIT

for trial in $(seq 1 "$trials"); do
  for value_size in 128 1024 10240; do
    for concurrency in 16 32 64 128; do
      for op in set get; do
        echo "trial=$trial value_size=$value_size concurrency=$concurrency op=$op"
        go run ./cmd/bench \
          -client "$client" -op "$op" -pipeline 1 \
          -n "$ops" -c "$concurrency" -value-size "$value_size" -warmup "$warmup" \
          | sed "s/}$/,\"trial\":$trial}/" \
          >> "results/payload-concurrency-${timestamp}.jsonl"
      done
    done
  done
done

# Profiling is deliberately separate from the throughput matrix because
# profiling adds overhead. These profiles describe the Go client process,
# not the Redis/Dragonfly server processes.
for op in set get; do
  go run ./cmd/bench \
    -client "$client" -op "$op" -pipeline 1 \
    -n 50000 -c 64 -value-size 10240 -warmup 5000 \
    -cpu-profile-prefix "results/client-cpu-${op}-10kb-c64-${timestamp}" \
    > "results/profile-run-${op}-${timestamp}.jsonl"
done

docker stats --no-stream > "results/container-stats-final-${timestamp}.txt" || true
