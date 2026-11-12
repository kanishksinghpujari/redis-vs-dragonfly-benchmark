# Redis vs Dragonfly: benchmark harness

A reproducible single-node benchmark harness comparing Redis OSS and Dragonfly using the same Go client and workload.

> **No results are claimed yet.** This repository contains the experiment harness. Run it on a documented machine and preserve raw output before publishing conclusions.

## Pinned server images

- Redis OSS: `redis:8.10.1`
- Dragonfly: `docker.dragonflydb.io/dragonflydb/dragonfly:v2.0.0`

Both tags are pinned in `docker-compose.yml`; avoid floating `latest` tags.

## Metrics

- SET/GET successful operations per second
- P50, P95, P99 and P99.9 client-observed latency
- Configurable operation count, concurrency, value size and warm-up
- Identical Go client implementation and workload against both servers

Client-library comparisons should be separate from this baseline so server and client effects are not conflated.

## Run

Requirements: Docker Compose and Go 1.22+.

```bash
docker compose up -d
docker compose ps

go run ./cmd/bench -op set -n 100000 -c 32 -value-size 128 -warmup 10000
go run ./cmd/bench -op get -n 100000 -c 32 -value-size 128 -warmup 10000

docker compose down
```

Save output:

```bash
mkdir -p results
go run ./cmd/bench -op set -n 100000 -c 32 -value-size 128 -warmup 10000 | tee results/set-$(date +%Y%m%d-%H%M%S).jsonl
```

Run at least five trials per configuration and report medians and spread, not the best run.

## Experimental protocol

1. Run both servers on the same host with identical CPU and memory limits.
2. Avoid competing workloads.
3. Use the same data set, key shape, payload, operation count, concurrency and client.
4. Warm up each server before measurement.
5. Record CPU, RAM, OS/kernel, Docker and Go versions, image digests, effective server configuration and exact commands.
6. Capture CPU and memory separately (for example with `docker stats`).
7. Keep the raw output alongside the article.

## Caveats

- Single-node only; no clustering, replication, failover or persistence tests.
- Latency is measured at the client and includes client scheduling/network overhead.
- This harness does not measure memory per key, recovery time, managed-service pricing or durability equivalence.
- Server defaults may differ; record effective settings before attributing differences to implementation.
- Run locally and inspect results before publishing. No benchmark has been run by CI.

## Layout

```
cmd/bench/          CLI
internal/benchmark/ workload runner and reporter (planned extension)
docker-compose.yml pinned server images
results/            raw trials and methodology notes
```

## References

- [Redis Docker image](https://hub.docker.com/_/redis)
- [Redis 8.10 release notes](https://redis.io/docs/latest/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.10-release-notes/)
- [Dragonfly releases](https://github.com/dragonflydb/dragonfly/releases)

## License

MIT
