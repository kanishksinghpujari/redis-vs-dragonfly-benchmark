# Benchmark results — 2026-11-12

Run: https://github.com/kanishksinghpujari/redis-vs-dragonfly-benchmark/actions/runs/43241244433

Raw JSONL and runner metadata are in the GitHub Actions artifact named `redis-dragonfly-benchmark-results` (retained by GitHub for 30 days).

## Environment

- GitHub-hosted Ubuntu Linux x86-64 runner
- AMD EPYC 7763; 4 vCPUs reported by the VM (2 cores / 2 threads per core); 15 GiB RAM
- Docker Engine 29.4.2; Docker Compose 5.1.3
- Go 1.25.14
- Redis OSS 8.10.3, image digest `sha256:1f88291b4c57bdf249f02eb085a8669889b2d2954e91d4de5118a5da6b78dd53`
- Dragonfly 2.0.0, image digest `sha256:7426fdb31ddcf7bd9499b4205f36ebaa83b26149ba1609a0d5f8f474b3631233`
- Both database containers limited to 1 vCPU and 2 GiB RAM
- 128-byte values; 16 benchmark workers; 3 trials per configuration
- 0 command errors across 108 result records

## Finding 1 — Explicit pipelining

Median throughput across three trials, commands/second:

| Operation | Batch | Redis | Dragonfly | Redis lead |
|---|---:|---:|---:|---:|
| GET | 1 | 41,360 | 35,552 | 16.3% |
| GET | 10 | 298,533 | 257,664 | 15.9% |
| GET | 50 | 675,542 | 605,847 | 11.5% |
| GET | 100 | 891,422 | 735,053 | 21.3% |
| SET | 1 | 38,865 | 34,819 | 11.6% |
| SET | 10 | 286,229 | 234,253 | 22.2% |
| SET | 50 | 581,700 | 517,281 | 12.5% |
| SET | 100 | 719,453 | 592,289 | 21.5% |

At batch size 100, throughput was about 21.6x the batch-size-1 GET rate on Redis and 20.7x on Dragonfly. For SET, the corresponding increases were about 18.5x and 17.0x.

**Latency interpretation:** for batch sizes above 1, reported P50/P95/P99/P99.9 values are batch round-trip times, not per-command latency. Do not compare those percentiles directly with single-command latency.

## Finding 2 — Client implementation

Median throughput, commands/second, pipeline size 1, automatic pipelining disabled for Rueidis:

| Database | Operation | go-redis 9.7.3 | Rueidis 1.0.79 | Rueidis change |
|---|---|---:|---:|---:|
| Redis | GET | 41,687 | 43,663 | +4.7% |
| Redis | SET | 39,507 | 43,086 | +9.1% |
| Dragonfly | GET | 36,471 | 37,040 | +1.6% |
| Dragonfly | SET | 35,231 | 37,270 | +5.8% |

These are results for this workload and runner only; they do not establish that one client is universally faster.

## Finding 3 — Added network response delay

The Toxiproxy cases all traverse a proxy, including the 0 ms case. The toxic adds downstream response delay; it is not a symmetric delay in both directions.

Median throughput, commands/second:

| Added downstream delay | Redis GET | Dragonfly GET | Redis SET | Dragonfly SET |
|---|---:|---:|---:|---:|
| 0 ms (proxy path) | 21,767 | 20,305 | 21,406 | 20,047 |
| 1 ms | 8,247 | 8,156 | 8,239 | 8,178 |
| 5 ms | 2,631 | 2,643 | 2,635 | 2,652 |

From the 0 ms proxy baseline to 1 ms, throughput fell by roughly 59–62%. At 5 ms, it fell by roughly 87–88%. At 5 ms, the Redis/Dragonfly throughput difference was below 1% in these runs.

## Interpretation

The data supports a narrow, defensible version of the headline: in this single-node, 128-byte, 16-worker workload, adding 1–5 ms of response-path delay had a much larger effect on throughput than the database choice. Pipelining also produced a large throughput increase for both systems. Redis was faster in most no-added-delay configurations, while the gap largely disappeared at 5 ms.

This does **not** prove that network is always the bottleneck. The experiment is synthetic and single-node; it does not cover production traffic mixes, larger values, persistence, replication, clustering, failover, CPU saturation analysis, or tail latency under mixed workloads.

## Reproduction

The exact raw trial output, machine metadata, image digests, and methodology are in the linked workflow artifact. Use the raw JSONL files for any further statistical analysis. The values above are medians across three trials, not averages and not vendor claims.
