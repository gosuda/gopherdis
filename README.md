<div align="center">

# Nedis

**A Redis-compatible in-memory store in pure Go that outperforms C Redis by up to 2.3x on multi-core hardware**

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

Measured on identical hardware and network conditions (AMD Ryzen 5 5600X, Debian Linux, 127.0.0.1 TCP socket), Nedis delivers **1.4x to 2.3x higher throughput** than C Redis 8.0 across seven workload types, from synthetic microbenchmarks to a 2M-key real-world cache load.

![C Redis 8.0 vs Nedis Standard Benchmark](benchmark_chart.svg?v=3)

![C Redis 8.0 vs Nedis SIMD Benchmark](benchmark_chart_simd.svg?v=3)

### Benchmark Summary

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) | SIMD Speedup | SIMD P99 / P99.9 vs C Redis |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| **Concurrent SET / GET** (50 Clients) | 132,205 ops/s | 309,445 ops/s | **304,610 ops/s** | 100,131 ops/s | **2.30x** (P50: 0.129ms) | 0.711ms +4% / 1.576ms +32% |
| **Multi-Field Hash** (5 Fields HSET/HMGET) | 118,447 ops/s | 196,183 ops/s | **184,902 ops/s** | 93,670 ops/s | **1.56x** (P50: 0.102ms) | 0.319ms +8% / 1.728ms +368% |
| **SkipList Leaderboard** (ZADD/ZRANK/ZRANGE) | 123,131 ops/s | 198,272 ops/s | **191,772 ops/s** | 616,610 ops/s † | **1.56x** (P50: 0.098ms) | 0.286ms **−1%** / 1.135ms +216% |
| **Stream Event Queue** (XADD/XRANGE) | 69,949 ops/s | 108,743 ops/s | **110,124 ops/s** | N/A | **1.57x** (P50: 0.141ms) | 0.777ms **−16%** / 1.307ms **−27%** |
| **Redlock Lua Script** (Bytecode JIT) | 115,297 ops/s | 156,623 ops/s | **156,793 ops/s** | N/A | **1.36x** (P50: 0.119ms) | 0.351ms +18% / 3.264ms +763% |
| **Bitmap & HyperLogLog** (POPCNT / Dense HLL) | 129,724 ops/s | 207,327 ops/s | **209,039 ops/s** | N/A | **1.61x** (P50: 0.095ms) | 0.192ms **−26%** / 0.634ms +84% |
| **Real-World Cache** (2M keys × 1KB) | 115,147 ops/s | 194,518 ops/s | **194,538 ops/s** | N/A ‡ | **1.69x** (P50: 0.127ms) | 1.211ms +47% / 2.291ms +47% |

Tail-latency percentages are versus the C Redis row of the same workload; negative (bold) is better. Nedis wins P99 on SET/GET, Hash, ZSet, and Bitmap workloads, but its P99.9 tail runs longer under GC pauses — the tradeoff for the throughput gains above.

### Memory Characteristics (`used_memory_rss` growth)

At realistic dataset sizes, the memory gap narrows to a bounded addend — 2M keys × 1KB values (~1.6GB payload):

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 2.06 GB | baseline |
| **Nedis (Standard)** | 3.00 GB | 1.46x |
| **Nedis (SIMD/AVX2)** | 2.99 GB | 1.46x |

On small overhead-dominated workloads (1–6), the fixed baseline differs more sharply: **6.25 MB (C Redis)** vs **75.75 MB (Nedis Standard)** total. The gap is deliberate pre-allocation (per-shard buffers, retained 8KB stream arena slabs) plus per-key storage amplification (2.0KB vs 1.3KB per 1KB pair) from shard bucket pre-sizing — constant with respect to dataset size, so it becomes proportionally smaller as data grows. See [BENCHMARK_REPORT.md §6](BENCHMARK_REPORT.md).

> † SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics and returns empty sets under this index-range workload, so its ZSet QPS is not apples-to-apples. SugarDB does not support Streams, Lua scripting, or Bitmap/HLL commands (N/A).
>
> ‡ SugarDB's server process crashes (container exit 2, goroutine dump) under the 2M×1KB real-world cache workload, so it is marked N/A there.
>
> Detailed methodology, latencies (P50/P95/P99/P99.9), and memory profiles are documented in [BENCHMARK_REPORT.md](BENCHMARK_REPORT.md). Against SugarDB, the leading pure-Go in-memory Redis alternative, Nedis is roughly **3.1x faster** on SET/GET (~309k vs ~100k ops/s) under the same harness. See [BENCHMARK_REPORT.md §5](BENCHMARK_REPORT.md).

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
