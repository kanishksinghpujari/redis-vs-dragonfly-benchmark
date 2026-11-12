# Redis vs Dragonfly: benchmark harness

A reproducible, single-node benchmark harness comparing **Redis OSS 8.10.3** and **Dragonfly 2.0.0** using the same Go client and workload.

> **No benchmark results are claimed yet.** This repository contains the harness and methodology. Run it on a documented machine and preserve raw output before publishing conclusions.

## Pinned server versions

- Redis OSS: `redis:8.10.3`
- Dragonfly: `docker.dragonflydb.io/dragonflydb/dragonfly:v2.0.0`

These are explicit version tags, not floating `latest` tags. Record the resolved image digests from your own machine with `docker image inspect` when publishing results.

## What it measures

- SET and GET throughput (successful operations/second)
- P50, P95, P99 and P99.9 client-observed latency
- Configurable operation count, concurrency, value size and warm-up count
- The same Go client implementation and workload against both servers

The baseline intentionally uses one client implementation so we don't conflate server differences with client-library differences. Client comparisons (such as go-redis versus valkey-go) should be a separate experiment.

## Run

Requirements: Docker Compose and Go 1.22+.

```bash
docker compose up -d --wait
docker compose ps

go run ./cmd/bench -op set -n 100000 -c 32 -value-size 128 -warmup 10000
go run ./cmd/bench -op get -n 100000 -c 32 -value-size 128 -warmup 10000

docker compose down
```

The CLI prints one JSON object per server (JSON Lines). Save raw output:

```bash
mkdir -p results
go run ./cmd/bench -op set -n 100000 -c 32 -value-size 128 -warmup 10000 \
  | tee results/set-$(date +%Y%m%d-%H%M%S).jsonl
```

Run at least five trials per configuration and report the median and spread, not the best run.

## Experiment matrix

Start with a matrix that varies one factor at a time:

| Workload | Operation | Concurrency | Value size |
|---|---|---:|---:|
| Baseline | SET / GET | 1, 8, 32, 64, 128 | 128 B |
| Payload | SET / GET | 32 | 16 B, 128 B, 1 KiB, 16 KiB |
| Saturation | SET / GET | 32, 64, 128 | 128 B |
| Warm connection | SET / GET | 32 | 128 B |

Use identical operation count and warm-up for compared trials. Do not run all configurations concurrently.

## Experimental discipline

1. Run both servers on the same host with identical CPU and memory limits.
2. Keep other heavy workloads off the host during trials.
3. Use the same key shape, payload size, operation count, concurrency and client.
4. Warm up before each measured run.
5. Repeat each configuration at least five times.
6. Record CPU model, physical/logical cores, RAM, OS/kernel, Docker and Go versions, image digests, effective server configuration and exact command.
7. Capture server CPU and memory separately (for example with `docker stats`).
8. Preserve raw JSONL output and include the experiment environment in the article.

## Interpretation and limitations

- This is a single-node comparison, not a cluster, replication or failover benchmark.
- Latency is measured by the client and includes client scheduling and network overhead.
- The harness does not currently measure persistence/durability, recovery time, memory per key, cloud pricing or managed-service behaviour.
- Matching container CPU/memory limits does not make every server default equivalent. Record configuration and disclose meaningful differences.
- Report errors alongside throughput; a fast run with failures is not a valid win.
- No benchmark has been executed by CI. Run and validate the harness locally before publishing findings.

## Layout

```
cmd/bench/          benchmark CLI
docker-compose.yml  pinned server images and comparable container limits
results/            methodology notes and raw trial output
scripts/            repeatable trial helpers
```

## Version references

- [Redis 8.10 release notes](https://redis.io/docs/latest/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.10-release-notes/)
- [Redis official Docker image tags](https://hub.docker.com/_/redis/)
- [Dragonfly v2.0.0 release](https://github.com/dragonflydb/dragonfly/releases/tag/v2.0.0)
- [Dragonfly container package versions](https://github.com/dragonflydb/dragonfly/pkgs/container/dragonfly/versions)

## License

MIT
