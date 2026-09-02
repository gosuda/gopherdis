<div align="center">

# Nedis

**A Redis-compatible in-memory store in pure Go that outperforms C Redis by up to 2.7x on multi-core hardware**

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

Measured on identical hardware and network conditions (AMD Ryzen 5 5600X, Debian Linux, 127.0.0.1 TCP socket), Nedis delivers **1.2x to 2.7x higher throughput** than C Redis 8.0 across seven workload types, from synthetic microbenchmarks to a 2M-key real-world cache load.

![C Redis 8.0 vs Nedis Standard Benchmark](benchmark_chart.svg?v=3)

![C Redis 8.0 vs Nedis SIMD Benchmark](benchmark_chart_simd.svg?v=3)

### Benchmark Summary

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) | SIMD Speedup | SIMD P99 / P99.9 vs C Redis |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| **Concurrent SET / GET** (50 Clients) | 111,124 ops/s | 310,806 ops/s | **295,038 ops/s** | 94,495 ops/s | **2.66x** (P50: 0.131ms) | 0.707ms **−18%** / 1.889ms +41% |
| **Multi-Field Hash** (5 Fields HSET/HMGET) | 114,675 ops/s | 173,996 ops/s | **177,535 ops/s** | 82,641 ops/s | **1.55x** (P50: 0.105ms) | 0.383ms +22% / 0.780ms +74% |
| **SkipList Leaderboard** (ZADD/ZRANK/ZRANGE) | 116,175 ops/s | 184,002 ops/s | **180,785 ops/s** | 594,594 ops/s † | **1.56x** (P50: 0.098ms) | 0.364ms +16% / 0.985ms +86% |
| **Stream Event Queue** (XADD/XRANGE) | 84,228 ops/s | 99,258 ops/s | **99,556 ops/s** | N/A | **1.18x** (P50: 0.152ms) | 0.952ms +94% / 2.091ms +80% |
| **Redlock Lua Script** (Bytecode JIT) | 111,134 ops/s | 116,181 ops/s | **131,999 ops/s** | N/A | **1.19x** (P50: 0.134ms) | 0.448ms +31% / 5.931ms +792% |
| **Bitmap & HyperLogLog** (POPCNT / Dense HLL) | 96,050 ops/s | 194,895 ops/s | **197,217 ops/s** | N/A | **2.05x** (P50: 0.099ms) | 0.234ms **−52%** / 1.147ms +4% |
| **Real-World Cache** (2M keys × 1KB) | 109,826 ops/s | 182,816 ops/s | **182,659 ops/s** | N/A ‡ | **1.66x** (P50: 0.154ms) | 1.294ms +47% / 2.818ms +71% |

Tail-latency percentages are versus the C Redis row of the same workload; negative (bold) is better. Nedis wins P99 on SET/GET, Hash, ZSet, and Bitmap workloads, but its P99.9 tail runs longer under GC pauses — the tradeoff for the throughput gains above.

### Memory Characteristics (`used_memory_rss` growth)

At realistic dataset sizes, Nedis and C Redis reach memory parity — 2M keys × 1KB values (~1.6GB payload):

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 2.06 GB | baseline |
| **Nedis (Standard)** | 1.82 GB | **0.88x** |
| **Nedis (SIMD/AVX2)** | 1.82 GB | **0.88x** |

Per-key cost is **1.2KB (Nedis) vs 1.4KB (C Redis)** per 1KB key/value pair: Go 1.24's Swiss-table maps and slim `Robj` headers keep per-key overhead below jemalloc's, and Nedis's `INFO memory` runs a full GC plus `debug.FreeOSMemory()` before reporting so GC headroom is not counted. On small overhead-dominated workloads (1–6) the fixed baseline is **5.88 MB (C Redis)** vs **26.62 MB (Nedis Standard)** — constant with respect to dataset size and negligible at scale. See [BENCHMARK_REPORT.md §6](BENCHMARK_REPORT.md).

> † SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics and returns empty sets under this index-range workload, so its ZSet QPS is not apples-to-apples. SugarDB does not support Streams, Lua scripting, or Bitmap/HLL commands (N/A).
>
> ‡ SugarDB's server process crashes (container exit 2, goroutine dump) under the 2M×1KB real-world cache workload, so it is marked N/A there.
>
> Detailed methodology, latencies (P50/P95/P99/P99.9), and memory profiles are documented in [BENCHMARK_REPORT.md](BENCHMARK_REPORT.md). Against SugarDB, the leading pure-Go in-memory Redis alternative, Nedis is roughly **3.3x faster** on SET/GET (~311k vs ~94k ops/s) under the same harness. See [BENCHMARK_REPORT.md §5](BENCHMARK_REPORT.md).

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
