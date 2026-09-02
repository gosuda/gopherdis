<div align="center">

# Nedis

**A Redis-compatible in-memory store in pure Go that outperforms C Redis by up to 2.4x on multi-core hardware**

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![Compatibility](https://img.shields.io/badge/Redis%20Compatibility-100%25%20(RESP2%20%26%20RESP3)-E10098?style=flat&logo=redis)](https://redis.io)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Zero CGo](https://img.shields.io/badge/CGo-Zero%20(Pure%20Go)-success)](#)

*Drop-in replacement for Redis. No CGo. No external runtime dependencies. Built for multi-core parallelism.*

</div>

---

## What is Nedis?

Nedis is an in-memory key-value store written in pure Go that speaks the exact same wire protocol as Redis (RESP2 and RESP3). Any existing Redis client, driver, or tool — including the official `redis-cli` — works against Nedis without modification.

It is designed for deployments where Redis-compatible semantics are required but a single-threaded event loop wastes the multi-core CPUs that modern servers provide.

## Why does it exist?

Official C Redis executes all commands on one thread, so throughput is capped by single-core performance regardless of how many cores the machine has. Nedis removes that ceiling while keeping full protocol compatibility:

- **64-shard architecture** — the keyspace is partitioned across 64 independent database shards, so reads and writes on different keys execute in parallel across all CPU cores without global lock contention.
- **Zero-allocation stream engine** — Streams use 8KB fixed chunk slabs from an unmanaged memory arena (`beaver/pure.Pool`), so `XADD`/`XRANGE` bypass Go heap allocations and avoid GC scan pauses that destabilize tail latency.
- **Bytecode-cached Lua engine** — scripts are compiled once into bytecode prototypes (`*lua.FunctionProto`) and executed concurrently on an elastic pool of isolated Lua VMs, instead of re-parsing source text per request under a global VM lock.
- **Predictive clustering** — standard 16,384 CRC16 hash slots with `-MOVED` redirection, plus a topology graph, an EWMA anomaly predictor that detects node exhaustion before failure, and shadow-master pre-provisioning for zero-downtime handover with epoch fencing tokens.

## How fast is it?

Measured on identical hardware and network conditions (AMD Ryzen 5 5600X, Debian Linux, 127.0.0.1 TCP socket), Nedis delivers **1.3x to 2.4x higher throughput** than C Redis 8.0 across six core workload types.

![C Redis 8.0 vs Nedis Standard Benchmark](benchmark_chart.svg?v=3)

![C Redis 8.0 vs Nedis SIMD Benchmark](benchmark_chart_simd.svg?v=3)

### Benchmark Summary

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) | SIMD Speedup | SIMD P99 / P99.9 (ms) |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| **Concurrent SET / GET** (50 Clients) | 133,211 ops/s | 296,553 ops/s | **312,358 ops/s** | 97,679 ops/s | **2.35x** (P50: 0.138ms) | 0.542 / 1.279 |
| **Multi-Field Hash** (5 Fields HSET/HMGET) | 116,930 ops/s | 152,937 ops/s | **189,946 ops/s** | 94,610 ops/s | **1.62x** (P50: 0.102ms) | 0.246 / 0.547 |
| **SkipList Leaderboard** (ZADD/ZRANK/ZRANGE) | 123,174 ops/s | 167,198 ops/s | **165,103 ops/s** | 656,722 ops/s † | **1.34x** (P50: 0.112ms) | 0.349 / 1.261 |
| **Stream Event Queue** (XADD/XRANGE) | 82,268 ops/s | 110,031 ops/s | **110,977 ops/s** | N/A | **1.35x** (P50: 0.139ms) | 0.838 / 1.645 |
| **Redlock Lua Script** (Bytecode JIT) | 112,896 ops/s | 155,053 ops/s | **154,827 ops/s** | N/A | **1.37x** (P50: 0.108ms) | 0.465 / 5.943 |
| **Bitmap & HyperLogLog** (POPCNT / Dense HLL) | 128,573 ops/s | 213,633 ops/s | **204,987 ops/s** | N/A | **1.59x** (P50: 0.092ms) | 0.244 / 1.204 |

> † SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics and returns empty sets under this index-range workload, so its ZSet QPS is not apples-to-apples. SugarDB does not support Streams, Lua scripting, or Bitmap/HLL commands (N/A).
>
> Detailed methodology, latencies (P50/P95/P99/P99.9), and memory profiles are documented in [BENCHMARK_REPORT.md](BENCHMARK_REPORT.md). Against SugarDB, the leading pure-Go in-memory Redis alternative, Nedis is roughly **3.0x faster** on SET/GET (~297k vs ~98k ops/s) under the same harness. See [BENCHMARK_REPORT.md §5](BENCHMARK_REPORT.md).

## What does it support?

- **Protocols**: Full RESP2 & RESP3 with seamless `HELLO 2/3` negotiation.
- **Data structures**: String, Hash, Quicklist, Set, Skiplist-backed ZSet, Bitmap (hardware POPCNT), 12KB Dense HLL (Otmar Ertl Sigma/Tau), 52-bit Geohash, Streams with Consumer Groups (PEL).
- **Scripting**: Lua with bytecode caching and concurrent multi-VM execution.
- **Persistence & HA**: Double-buffered non-blocking AOF, RDB v9 LZF snapshots, master-replica sync, and a Redis Sentinel facade adapter.
- **Clustering**: 16,384 hash slots, `-MOVED` redirection, predictive failure detection, zero-downtime failover.

## How do I run it?

### Docker (GHCR)

```bash
# Standard edition
docker run -d --name nedis -p 6379:6379 ghcr.io/gosuda/nedis:latest

# Hardware-accelerated SIMD (AVX2) edition
docker run -d --name nedis-simd -p 6379:6379 ghcr.io/gosuda/nedis:simd

# Or with docker compose
docker compose up -d
```

### Build from source

```bash
git clone https://github.com/gosuda/nedis.git
cd nedis

# Standard edition
go build -o bin/nedis-server ./cmd/nedis-server

# Hardware-accelerated SIMD (AVX2) edition
GOAMD64=v3 go build -o bin/nedis-server-simd ./cmd/nedis-server
```

### Start and connect

```bash
./bin/nedis-server --port 6379
```

```bash
redis-cli -p 6379 PING
# PONG

redis-cli -p 6379 SET mykey "Hello from Nedis"
# OK

redis-cli -p 6379 GET mykey
# "Hello from Nedis"
```

## How is it verified?

```bash
# Unit and integration tests with the Go race detector
go test -count=1 -race ./...

# Live 1:1 differential comparison against real C Redis
go test -v ./tests/compat_test.go

# 6-dimensional benchmark suite
go run ./benchmarks/benchmark_suite.go
```

## License

MIT License. Copyright (c) 2026 gosuda.
