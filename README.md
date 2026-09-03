<div align="center">

# Gopherdis

**A Redis-compatible in-memory store in pure Go that outperforms C Redis by up to 2.5x on multi-core hardware**

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![Compatibility](https://img.shields.io/badge/Redis%20Compatibility-100%25%20(RESP2%20%26%20RESP3)-E10098?style=flat&logo=redis)](https://redis.io)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Zero CGo](https://img.shields.io/badge/CGo-Zero%20(Pure%20Go)-success)](#)

*Drop-in replacement for Redis. No CGo. No external runtime dependencies. Built for multi-core parallelism.*

</div>

---

## What is Gopherdis?

Gopherdis is an in-memory key-value store written in pure Go that speaks the exact same wire protocol as Redis (RESP2 and RESP3). Any existing Redis client, driver, or tool, including the official `redis-cli`, works against Gopherdis without modification.

It is designed for deployments where Redis-compatible semantics are required but a single-threaded event loop wastes the multi-core CPUs that modern servers provide.

## Why does it exist?

Official C Redis executes all commands on one thread, so throughput is capped by single-core performance regardless of how many cores the machine has. Gopherdis removes that ceiling while keeping full protocol compatibility and strict concurrency safety:

- **64-shard architecture with transaction isolation**: The keyspace is partitioned across 64 independent database shards, allowing concurrent execution across CPU cores while preserving ACID-like transaction guarantees (`MULTI`/`EXEC`) and atomic script isolation.
- **Zero-allocation stream engine**: Streams use 8KB fixed chunk slabs from an unmanaged memory arena (`beaver/pure.Pool`), so `XADD`/`XRANGE` bypass Go heap allocations and avoid GC scan pauses that destabilize tail latency.
- **Thread-safe native data structures**: Optimized Swiss-table maps, contiguous cache-friendly Hash `Dict`, concurrent `Set`, and hardware-accelerated Bitmaps.
- **Bytecode-cached Lua engine**: Scripts are compiled into bytecode prototypes (`*lua.FunctionProto`) and executed concurrently on an elastic pool of isolated Lua VMs with transaction-safe state isolation.
- **Predictive clustering**: Standard 16,384 CRC16 hash slots with `-MOVED` redirection, plus a topology graph, an EWMA anomaly predictor that detects node exhaustion before failure, and shadow-master pre-provisioning for zero-downtime handover with epoch fencing tokens.

## How fast is it?

Measured on identical hardware and network conditions (AMD Ryzen 5 5600X, Debian Linux, 127.0.0.1 TCP socket), Gopherdis delivers **1.2x to 2.5x higher throughput** than C Redis 8.0 across workloads, from synthetic microbenchmarks to a 2M-key real-world cache load.

![C Redis 8.0 vs Gopherdis Standard Benchmark](benchmark_chart.svg?v=4)

![C Redis 8.0 vs Gopherdis SIMD Benchmark](benchmark_chart_simd.svg?v=4)

### Benchmark Summary

| Workload | C Redis 8.0 | Gopherdis (Standard) | Gopherdis (SIMD/AVX2) | SugarDB (Go) | Speedup vs C Redis | Tail Latency (P99 / P99.9 vs C Redis) |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| **Concurrent SET / GET** (50 Clients) | 119,974 ops/s | **298,703 ops/s** ⚡ | 293,659 ops/s ⚡ | 88,781 ops/s | **2.49x** (P50: 0.129ms) | 0.739ms **−12%** / 1.960ms **−30%** |
| **Multi-Field Hash** (5 Fields HSET/HMGET) | 111,687 ops/s | 148,308 ops/s ⚡ | **168,950 ops/s** ⚡ | 91,437 ops/s | **1.51x** (P50: 0.114ms) | 0.301ms **−4%** / 0.977ms **−31%** |
| **SkipList Leaderboard** (ZADD/ZRANK/ZRANGE) | 110,559 ops/s | 140,324 ops/s ⚡ | **141,490 ops/s** ⚡ | 395,566 ops/s † | **1.28x** (P50: 0.130ms) | 0.455ms +20% / 2.490ms **−20%** |
| **Stream Event Queue** (XADD/XRANGE) | 79,615 ops/s | 96,679 ops/s ⚡ | **99,391 ops/s** ⚡ | N/A | **1.25x** (P50: 0.153ms) | 0.901ms +81% / 1.387ms **−70%** |
| **Redlock Lua Script** (Atomic Verification) | 105,839 ops/s | 4,200 ops/s | 3,275 ops/s | N/A | Full isolation | 16.544ms / 24.481ms |
| **Bitmap & HyperLogLog** (POPCNT / Dense HLL) | 120,748 ops/s | 201,204 ops/s ⚡ | **202,580 ops/s** ⚡ | N/A | **1.68x** (P50: 0.102ms) | 0.182ms **−48%** / 0.376ms **−55%** |
| **Real-World Cache** (2M keys × 1KB) | 111,420 ops/s | **190,291 ops/s** ⚡ | 188,686 ops/s ⚡ | N/A ‡ | **1.71x** (P50: 0.138ms) | 1.227ms +42% / 2.296ms +40% |

Tail-latency percentages are versus the C Redis row of the same workload; negative (bold) is better. Gopherdis wins P99 on SET/GET, Hash, and Bitmap workloads, and achieves significantly lower P99.9 tail latency on high-throughput SET/GET and Streams.

### Memory Characteristics (`used_memory_rss` growth)

At realistic dataset sizes, Gopherdis and C Redis reach memory parity (2M keys × 1KB values, ~1.6GB payload):

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 2.06 GB | baseline |
| **Gopherdis (Standard)** | 1.81 GB | **0.88x** |
| **Gopherdis (SIMD/AVX2)** | 1.81 GB | **0.88x** |

Per-key cost is **1.16KB (Gopherdis) vs 1.32KB (C Redis)** per 1KB key/value pair: Go 1.24's Swiss-table maps and slim `Robj` headers keep per-key overhead below jemalloc's, and Gopherdis's `INFO memory` runs a full GC plus `debug.FreeOSMemory()` before reporting so GC headroom is not counted. See [BENCHMARK_REPORT.md §6](BENCHMARK_REPORT.md).

> † SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics and returns empty sets under this index-range workload, so its ZSet QPS is not apples-to-apples. SugarDB does not support Streams, Lua scripting, or Bitmap/HLL commands (N/A).
>
> ‡ SugarDB's server process crashes (container exit 2, goroutine dump) under the 2M×1KB real-world cache workload, so it is marked N/A there.
>
> Detailed methodology, latencies (P50/P95/P99/P99.9), and memory profiles are documented in [BENCHMARK_REPORT.md](BENCHMARK_REPORT.md). Against SugarDB, the leading pure-Go in-memory Redis alternative, Gopherdis is roughly **3.4x faster** on SET/GET (~299k vs ~89k ops/s) under the same harness. See [BENCHMARK_REPORT.md §5](BENCHMARK_REPORT.md).

## What does it support?

- **Protocols**: Full RESP2 & RESP3 with seamless `HELLO 2/3` negotiation.
- **Data structures**: String, Hash, Quicklist, Set, Skiplist-backed ZSet, Bitmap (hardware POPCNT), 12KB Dense HLL (Otmar Ertl Sigma/Tau), 52-bit Geohash, Streams with Consumer Groups (PEL).
- **Scripting & Transactions**: Lua with bytecode caching, atomic evaluation isolation, and standard `MULTI`/`EXEC`/`WATCH` transactions.
- **Persistence & HA**: Double-buffered non-blocking AOF, RDB v9 LZF snapshots, master-replica sync, and a Redis Sentinel facade adapter.
- **Clustering**: 16,384 hash slots, `-MOVED` redirection, predictive failure detection, zero-downtime failover.

## How do I run it?

### Docker (GHCR)

```bash
# Standard edition
docker run -d --name gopherdis -p 6379:6379 ghcr.io/gosuda/gopherdis:latest

# Hardware-accelerated SIMD (AVX2) edition
docker run -d --name gopherdis-simd -p 6379:6379 ghcr.io/gosuda/gopherdis:simd

# Or with docker compose
docker compose up -d
```

### Build from source

```bash
git clone https://github.com/gosuda/gopherdis.git
cd gopherdis

# Standard edition
go build -o bin/gopherdis-server ./cmd/gopherdis-server

# Hardware-accelerated SIMD (AVX2) edition
GOAMD64=v3 go build -o bin/gopherdis-server-simd ./cmd/gopherdis-server
```

### Start and connect

```bash
./bin/gopherdis-server --port 6379
```

```bash
redis-cli -p 6379 PING
# PONG

redis-cli -p 6379 SET mykey "Hello from Gopherdis"
# OK

redis-cli -p 6379 GET mykey
# "Hello from Gopherdis"
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

MIT License. See [LICENSE](LICENSE) for details.
