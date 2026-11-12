# Redis vs Dragonfly: reproducible benchmark

A reproducible, single-node benchmark suite comparing **Redis OSS 8.10.3** and **Dragonfly 2.0.0** with controlled Go workloads.

> **No performance findings are pre-filled.** Run the experiments on your own documented hardware and publish the raw JSONL files with any conclusions. The harness is code, not evidence until it has been run.

## Pinned versions

- Redis OSS: `redis:8.10.3`
- Dragonfly: `docker.dragonflydb.io/dragonflydb/dragonfly:v2.0.0`
- go-redis: `github.com/redis/go-redis/v9 v9.7.3`
- Rueidis: `github.com/redis/rueidis v1.0.79`

The Rueidis comparison disables its automatic pipelining and client-side cache. This isolates the direct-command path; it is not a comparison of each library's maximum-throughput defaults. Rueidis also supports manual `DoMulti` batching, used by the pipeline experiment.

## Requirements

- Docker Compose
- Go 1.25+
- curl (for the Toxiproxy latency experiment)

## Quick baseline

```bash
docker compose up -d --wait
go run ./cmd/bench -op set -n 100000 -c 32 -value-size 128 -warmup 10000
go run ./cmd/bench -op get -n 100000 -c 32 -value-size 128 -warmup 10000
docker compose down
```

Each invocation emits one JSON object per server. Fields include throughput, errors, concurrency, client, pipeline size and latency percentiles.

## Experiment 1: explicit pipelining

Run batch sizes 1, 10, 50 and 100:

```bash
./scripts/run-pipeline-matrix.sh
```

The benchmark records P50/P95/P99/P99.9 of **batch round-trip time**, not per-command latency, when pipeline size is greater than one. Throughput is successful commands per second. This distinction matters: do not label a batch RTT percentile as an individual operation's latency.

## Experiment 2: artificial network latency

The Compose stack includes Toxiproxy. The script routes both databases through equivalent proxies and adds a downstream response delay of 0, 1 or 5 ms:

```bash
./scripts/run-network-latency.sh
```

The toxic is a one-way downstream delay, so it adds approximately the configured amount to each request's response path, not a symmetric delay in both directions. The 0 ms case still passes through Toxiproxy; compare the three proxy cases with each other, not directly with the no-proxy baseline.

## Experiment 3: client overhead

Compare go-redis and Rueidis with explicit pipeline size 1:

```bash
./scripts/run-client-matrix.sh
```

Rueidis automatic pipelining and client-side caching are disabled in this direct-command comparison. The goal is to avoid comparing go-redis direct calls with Rueidis' auto-batched path. A separate experiment can later measure each library's default/optimized mode, but that would answer a different question.

## Run a custom configuration

```bash
go run ./cmd/bench \
  -client go-redis \
  -op set \
  -pipeline 50 \
  -n 100000 \
  -c 32 \
  -value-size 128 \
  -warmup 10000
```

Options:

| Flag | Default | Meaning |
|---|---:|---|
| `-client` | go-redis | go-redis or rueidis |
| `-op` | set | set or get |
| `-pipeline` | 1 | Commands per request/round trip |
| `-n` | 100000 | Measured commands per server |
| `-c` | 32 | Concurrent workers |
| `-value-size` | 128 | Payload bytes |
| `-warmup` | 10000 | Warm-up operations |
| `-redis-addr` | localhost:6379 | Redis endpoint |
| `-dragonfly-addr` | localhost:6380 | Dragonfly endpoint |

## Experimental protocol

1. Use the same host, CPU/memory limits, dataset shape, key count, payload, command count and concurrency.
2. Run one experiment at a time; avoid other heavy workloads.
3. Run at least five trials per configuration and report median plus spread.
4. Capture CPU and memory independently (e.g. `docker stats`).
5. Record CPU model, core count, RAM, OS/kernel, Docker and Go versions, image digests, effective server configuration and exact commands.
6. Preserve every raw result, including failed trials; report errors.
7. Randomize or alternate which server runs first in repeated trials to reduce warm-cache/order bias.
8. For latency experiments, document that Toxiproxy adds downstream delay and that all compared cases use the same proxy path.

## Important limitations

- Single-node only; no replication, cluster, failover, persistence or recovery tests.
- Client-observed latency includes client scheduling, serialization and network overhead.
- Batch percentiles are batch round-trip times when pipeline size is greater than one.
- Container CPU/memory limits do not guarantee equivalent internal tuning. Record effective configuration.
- The client comparison is limited to direct-command mode with Rueidis auto-pipelining disabled; do not generalize it to each library's maximum throughput.
- No benchmark has been run by CI. CI verifies build, vet and unit tests only.

## Repository layout

```
cmd/bench/                       Go benchmark CLI and unit tests
scripts/run-pipeline-matrix.sh   batch size sweep
scripts/run-network-latency.sh   Toxiproxy delay sweep
scripts/run-client-matrix.sh     client implementation comparison
docker-compose.yml               pinned databases and Toxiproxy
results/                          raw output and methodology notes
```

## References

- [Redis 8.10 release notes](https://redis.io/docs/latest/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.10-release-notes/)
- [Redis official Docker image](https://hub.docker.com/_/redis/)
- [Dragonfly releases](https://github.com/dragonflydb/dragonfly/releases/tag/v2.0.0)
- [Rueidis v1.0.79](https://github.com/redis/rueidis/releases/tag/v1.0.79)
- [Toxiproxy](https://github.com/Shopify/toxiproxy)

## License

MIT
