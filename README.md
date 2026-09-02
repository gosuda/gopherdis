<div align="center">

# Nedis

**A Redis-compatible in-memory store in pure Go that outperforms C Redis by up to 2.6x on multi-core hardware**

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

Measured on identical hardware and network conditions (AMD Ryzen 5 5600X, Debian Linux, 127.0.0.1 TCP socket), Nedis delivers **1.2x to 2.6x higher throughput** than C Redis 8.0 across six core workload types.

![C Redis 8.0 vs Nedis Standard Benchmark](benchmark_chart.svg?v=3)

![C Redis 8.0 vs Nedis SIMD Benchmark](benchmark_chart_simd.svg?v=3)

### Benchmark Summary

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) | SIMD Speedup | SIMD P99 / P99.9 (ms) |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| **Concurrent SET / GET** (50 Clients) | 123,923 ops/s | 313,256 ops/s | **321,851 ops/s** | 97,855 ops/s | **2.60x** (P50: 0.127ms) | 0.588 / 1.452 |
| **Multi-Field Hash** (5 Fields HSET/HMGET) | 120,440 ops/s | 189,238 ops/s | **196,599 ops/s** | 97,240 ops/s | **1.63x** (P50: 0.100ms) | 0.235 / 0.572 |
| **SkipList Leaderboard** (ZADD/ZRANK/ZRANGE) | 122,047 ops/s | 202,417 ops/s | **198,623 ops/s** | 619,799 ops/s † | **1.63x** (P50: 0.093ms) | 0.254 / 1.428 |
| **Stream Event Queue** (XADD/XRANGE) | 87,330 ops/s | 108,872 ops/s | **111,162 ops/s** | N/A | **1.27x** (P50: 0.138ms) | 0.829 / 1.735 |
| **Redlock Lua Script** (Bytecode JIT) | 110,752 ops/s | 78,893 ops/s | **136,547 ops/s** | N/A | **1.23x** (P50: 0.133ms) | 0.394 / 5.186 |
| **Bitmap & HyperLogLog** (POPCNT / Dense HLL) | 133,526 ops/s | 206,107 ops/s | **205,275 ops/s** | N/A | **1.54x** (P50: 0.098ms) | 0.193 / 0.587 |

### Memory Characteristics (`used_memory` growth per workload)

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) |
|---|:---:|:---:|:---:|:---:|
| **Concurrent SET / GET** | 4.90 MB | 11.72 MB | 12.40 MB | 0 B ‡ |
| **Multi-Field Hash** | 1.84 MB | 13.60 MB | 12.62 MB | 0 B ‡ |
| **SkipList Leaderboard** | 282.78 KB | 0 B | 0 B | 0 B ‡ |
| **Stream Event Queue** | 407.17 KB | 10.56 MB | 10.82 MB | N/A |
| **Redlock Lua Script** | 22.16 KB | 0 B | 155.13 KB | N/A |
| **Bitmap & HyperLogLog** | 112.98 KB | 9.96 MB | 9.91 MB | N/A |
| **Total** | **7.55 MB** | **45.84 MB** | **45.90 MB** | **0 B** ‡ |

> ‡ SugarDB does not implement `INFO memory`, so its memory usage cannot be measured in this suite. Nedis's higher delta is deliberate pre-allocation (per-shard buffers, retained 8KB stream arena slabs), not per-request leakage. See [BENCHMARK_REPORT.md §6](BENCHMARK_REPORT.md).

> † SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics and returns empty sets under this index-range workload, so its ZSet QPS is not apples-to-apples. SugarDB does not support Streams, Lua scripting, or Bitmap/HLL commands (N/A).
>
> Detailed methodology, latencies (P50/P95/P99/P99.9), and memory profiles are documented in [BENCHMARK_REPORT.md](BENCHMARK_REPORT.md). Against SugarDB, the leading pure-Go in-memory Redis alternative, Nedis is roughly **3.2x faster** on SET/GET (~313k vs ~98k ops/s) under the same harness. See [BENCHMARK_REPORT.md §5](BENCHMARK_REPORT.md).

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
