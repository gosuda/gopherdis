<div align="center">

# Nedis

**A Redis-compatible in-memory store in pure Go that outperforms C Redis by up to 2.5x on multi-core hardware**

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

Measured on identical hardware and network conditions (AMD Ryzen 5 5600X, Debian Linux, 127.0.0.1 TCP socket), Nedis delivers **1.3x to 2.5x higher throughput** than C Redis 8.0 across seven workload types, from synthetic microbenchmarks to a 500k-key real-world cache load.

![C Redis 8.0 vs Nedis Standard Benchmark](benchmark_chart.svg?v=3)

![C Redis 8.0 vs Nedis SIMD Benchmark](benchmark_chart_simd.svg?v=3)

### Benchmark Summary

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) | SIMD Speedup | SIMD P99 / P99.9 (ms) |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| **Concurrent SET / GET** (50 Clients) | 129,591 ops/s | 313,822 ops/s | **319,249 ops/s** | 105,872 ops/s | **2.46x** (P50: 0.129ms) | 0.635 / 1.354 |
| **Multi-Field Hash** (5 Fields HSET/HMGET) | 115,000 ops/s | 189,069 ops/s | **189,965 ops/s** | 93,974 ops/s | **1.65x** (P50: 0.102ms) | 0.258 / 0.553 |
| **SkipList Leaderboard** (ZADD/ZRANK/ZRANGE) | 118,036 ops/s | 169,995 ops/s | **198,830 ops/s** | 637,205 ops/s † | **1.68x** (P50: 0.095ms) | 0.278 / 0.614 |
| **Stream Event Queue** (XADD/XRANGE) | 87,646 ops/s | 111,670 ops/s | **111,107 ops/s** | N/A | **1.27x** (P50: 0.144ms) | 0.798 / 1.433 |
| **Redlock Lua Script** (Bytecode JIT) | 111,578 ops/s | 156,551 ops/s | **147,882 ops/s** | N/A | **1.33x** (P50: 0.119ms) | 0.406 / 5.754 |
| **Bitmap & HyperLogLog** (POPCNT / Dense HLL) | 131,605 ops/s | 212,911 ops/s | **210,141 ops/s** | N/A | **1.60x** (P50: 0.095ms) | 0.181 / 0.396 |
| **Real-World Cache** (500k keys × 1KB) | 117,916 ops/s | 193,763 ops/s | **193,823 ops/s** | N/A ‡ | **1.64x** (P50: 0.129ms) | 1.203 / 2.103 |

### Memory Characteristics (`used_memory_rss` growth)

At realistic dataset sizes, the memory gap narrows to a bounded addend — 500k keys × 1KB values (~512MB payload):

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 523.18 MB | baseline |
| **Nedis (Standard)** | 814.17 MB | 1.56x |
| **Nedis (SIMD/AVX2)** | 863.48 MB | 1.65x |

On small overhead-dominated workloads (1–6), the fixed baseline differs more sharply: **6.75 MB (C Redis)** vs **71.82 MB (Nedis Standard)** total. The gap is deliberate pre-allocation (per-shard buffers, retained 8KB stream arena slabs) plus ~1.9KB-per-1KB-pair storage amplification from shard bucket pre-sizing — constant with respect to dataset size, so it becomes proportionally smaller as data grows. See [BENCHMARK_REPORT.md §6](BENCHMARK_REPORT.md).

> † SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics and returns empty sets under this index-range workload, so its ZSet QPS is not apples-to-apples. SugarDB does not support Streams, Lua scripting, or Bitmap/HLL commands (N/A).
>
> ‡ SugarDB's server process crashes (container exit 2, goroutine dump) under the 500k×1KB real-world cache workload, so it is marked N/A there.
>
> Detailed methodology, latencies (P50/P95/P99/P99.9), and memory profiles are documented in [BENCHMARK_REPORT.md](BENCHMARK_REPORT.md). Against SugarDB, the leading pure-Go in-memory Redis alternative, Nedis is roughly **3.0x faster** on SET/GET (~314k vs ~106k ops/s) under the same harness. See [BENCHMARK_REPORT.md §5](BENCHMARK_REPORT.md).

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

# 7-dimensional benchmark suite
go run ./benchmarks/benchmark_suite.go
```

## License

MIT License. Copyright (c) 2026 gosuda.
