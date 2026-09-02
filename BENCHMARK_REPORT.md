# 📊 C Redis 8.0 vs Nedis (Pure Go) vs SugarDB (Pure Go) 6-Dimensional Benchmark Analysis Report

This document details the multi-dimensional benchmark methodology and performance comparison results between official C Redis 8.0, Nedis (Pure Go Redis-compatible store), and SugarDB (Pure Go in-memory Redis alternative, running in Docker with host networking), executed under an identical hardware and local loopback TCP network environment.

## 1. ⚙️ System Environment & Test Setup

| Parameter | Specification |
|---|---|
| **CPU** | AMD Ryzen 5 5600X 6-Core Processor (6 Cores / 12 Threads, Base 3.7GHz ~ Boost 4.6GHz) |
| **RAM** | 64 GB DDR4 (~38 GB available) |
| **Operating System & Kernel** | Debian GNU/Linux (Trixie/Sid), Kernel `6.12.101+deb13-amd64` (SMP PREEMPT_DYNAMIC) |
| **Go Compiler** | `go version go1.24.4 linux/amd64` |
| **C Redis Version** | Redis server v=8.0.2 (sha=00000000:0, malloc=jemalloc-5.3.0, 64-bit) |
| **SugarDB Version** | `echovault/sugardb:latest` (Docker, host networking, port 16381) |
| **Network Interface** | Local Loopback TCP (`127.0.0.1:16379` / `:16380` / `:16381` / `:16382`) |
| **Benchmark Protocol** | RESP2 / RESP3 direct TCP socket streaming with pre-connected connection pooling |

## 2. 📈 Performance Visualization

![C Redis vs Nedis Benchmark Chart](benchmark_chart.svg)

## 3. 📋 Benchmark Summary Table

| Workload Scenario | Target Engine | Total Ops | Throughput (QPS) | P50 Latency (ms) | P95 Latency (ms) | P99 Latency (ms) | Memory Delta |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | 50000 | **126808 ops/s** | 0.374 ms | 0.485 ms | **0.728 ms** | 4.90 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | 50000 | **319215 ops/s** ⚡ | 0.125 ms | 0.329 ms | **0.647 ms** | 12.97 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | 50000 | **313409 ops/s** ⚡ | 0.129 ms | 0.320 ms | **0.626 ms** | 12.77 MB |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | 50000 | **90102 ops/s** | 0.269 ms | 1.514 ms | **2.185 ms** | 0 B |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | 20000 | **115388 ops/s** | 0.167 ms | 0.231 ms | **0.314 ms** | 1.84 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | 20000 | **195558 ops/s** ⚡ | 0.099 ms | 0.159 ms | **0.240 ms** | 12.27 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | 20000 | **191316 ops/s** ⚡ | 0.103 ms | 0.156 ms | **0.227 ms** | 13.37 MB |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | 20000 | **85073 ops/s** | 0.153 ms | 0.506 ms | **0.820 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | 20000 | **121979 ops/s** | 0.154 ms | 0.215 ms | **0.285 ms** | 282.55 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | 20000 | **194470 ops/s** ⚡ | 0.097 ms | 0.165 ms | **0.282 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | 20000 | **193523 ops/s** ⚡ | 0.094 ms | 0.174 ms | **0.310 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | 20000 | **592004 ops/s** | 0.009 ms | 0.010 ms | **0.024 ms** | 0 B |
| 4. Stream Queue (XADD/XRANGE) | **C Redis 8.0** | 20000 | **87521 ops/s** | 0.216 ms | 0.348 ms | **0.448 ms** | 407.11 KB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go)** | 20000 | **107709 ops/s** ⚡ | 0.142 ms | 0.502 ms | **0.861 ms** | 9.60 MB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go SIMD)** | 20000 | **108301 ops/s** ⚡ | 0.145 ms | 0.476 ms | **0.807 ms** | 10.58 MB |
| 4. Stream Queue (XADD/XRANGE) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — |
| 5. Redlock Atomic Lua Scripting | **C Redis 8.0** | 20000 | **110804 ops/s** | 0.173 ms | 0.240 ms | **0.322 ms** | 22.16 KB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go)** | 20000 | **130485 ops/s** ⚡ | 0.128 ms | 0.226 ms | **0.438 ms** | 0 B |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go SIMD)** | 20000 | **134544 ops/s** ⚡ | 0.133 ms | 0.207 ms | **0.375 ms** | 0 B |
| 5. Redlock Atomic Lua Scripting | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — |
| 6. Bitmap & HyperLogLog | **C Redis 8.0** | 30000 | **130965 ops/s** | 0.142 ms | 0.196 ms | **0.259 ms** | 116.98 KB |
| 6. Bitmap & HyperLogLog | **Nedis (Go)** | 30000 | **213186 ops/s** ⚡ | 0.093 ms | 0.140 ms | **0.179 ms** | 9.94 MB |
| 6. Bitmap & HyperLogLog | **Nedis (Go SIMD)** | 30000 | **215248 ops/s** ⚡ | 0.092 ms | 0.142 ms | **0.194 ms** | 9.91 MB |
| 6. Bitmap & HyperLogLog | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — |

> **Notes**: SugarDB does not support Streams (XADD/XRANGE), Lua scripting (EVAL/SCRIPT LOAD), Bitmaps (SETBIT/BITCOUNT), or HyperLogLog (PFADD) — verified empirically via `redis-cli` error replies — so those workloads are marked N/A. SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics rather than index-range, so its ZSet range queries return empty sets under this workload; the row is still measured. SugarDB does not support `INFO memory`, so its Memory Delta is reported as 0 B.

---

## 4. 🔍 Deep-Dive Analysis by Scenario

### ① High-Concurrency SET/GET Throughput
- **Scenario**: 50 concurrent client connections, 50,000 operations with 128-byte payloads.
- **Analysis**: Nedis utilizes a **64-shard contention-free architecture**, socket-level `TCP_NODELAY`, and Beaver Arena memory pooling across 12 CPU hardware threads. It achieves **319k QPS (2.52x higher than C Redis)** with comparable tail latency (**P99: 0.647ms vs C Redis 0.728ms**). SugarDB reaches 90k QPS (Nedis is 3.54x faster).

### ② Multi-Field Hash Aggregation
- **Scenario**: 20 concurrent clients, 5-field HSET and HMGET operations across 20,000 requests.
- **Analysis**: With **Hybrid Flat-Dict (contiguous array pairs for <= 64 entries + automatic hash map promotion)** and single-pass RESP buffer serialization, Nedis delivers **196k QPS (1.69x C Redis)**, **P50 of 0.099ms**, and **P99 of 0.240ms (vs C Redis 0.314ms)**. SugarDB reaches 85k QPS (Nedis is 2.30x faster).

### ③ Ranked SkipList Leaderboard (ZSet)
- **Scenario**: 2,000 simulated players with real-time score updates (ZADD), rank lookups (ZRANK), and top-N range queries (ZRANGE).
- **Analysis**: Lightweight Mutex synchronization, lock-free `math/rand/v2` level generation, and stack-allocated node arrays achieve **194k QPS (1.59x C Redis)**, with **P95 (0.165ms vs 0.215ms)** and **P99 (0.282ms vs 0.285ms)**. (SugarDB's ZRANGE uses score-range semantics and returns empty sets here, so its 592k QPS is not an apples-to-apples comparison.)

### ④ Stream Event Queue (XADD / XRANGE)
- **Scenario**: 20 concurrent producers and consumers logging sensor telemetry events and querying ranges across 20,000 records.
- **Analysis**: O(1) chunk boundary skipping and `AddRaw` zero-copy parsing yield **107.7k QPS (1.23x C Redis)** with **P50 of 0.142ms** and **P95 of 0.502ms (vs C Redis 0.348ms)**. SugarDB does not support Streams (N/A).

### ⑤ Redlock Atomic Lua Scripting (Bytecode JIT Cache)
- **Scenario**: 20 workers competing for 100 distributed lock keys with atomic Lua release scripts.
- **Analysis**: Pre-compiled `FunctionProto` caching, VM table reuse, and zero-alloc `redis.call` argument conversions produce **130.5k QPS (1.18x C Redis)** with **P50 of 0.128ms**, **P95 of 0.226ms**, and **P99 of 0.438ms (vs C Redis 0.322ms)**. SugarDB does not support EVAL/SCRIPT LOAD (N/A).

### ⑥ Bitmap & HyperLogLog Cardinality Estimation
- **Scenario**: 100,000 bit mutations with SETBIT, 64-bit word POPCNT BITCOUNT, and 50,000 unique IP insertions with PFADD/PFCOUNT.
- **Analysis**: In-place zero-reallocation bit mutation, `math/bits.OnesCount64` hardware acceleration, and 12KB Dense Otmar Ertl HLL registers achieve **213.2k QPS (1.63x C Redis)** with **P50 of 0.093ms** and **P95 of 0.140ms (vs C Redis 0.196ms)**. SugarDB does not support SETBIT/BITCOUNT/PFADD (N/A).

---

## 5. 🐹 Go Implementation Comparison (vs SugarDB)

SugarDB is now measured as a first-class target in this suite (see §3), so the numbers below are taken directly from the suite-measured rows above — identical machine (AMD Ryzen 5 5600X), identical harness (direct RESP TCP, pre-connected pooled clients), SugarDB `latest` running in Docker with host networking. SugarDB supports only workloads 1–3; workloads 4–6 are N/A (unsupported).

| Workload | Target | QPS (ops/s) | P50 (ms) | P95 (ms) | P99 (ms) |
|---|---|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | **126808** | 0.374 | 0.485 | 0.728 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | **319215** | 0.125 | 0.329 | 0.647 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | **313409** | 0.129 | 0.320 | 0.626 |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | **90102** | 0.269 | 1.514 | 2.185 |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | **115388** | 0.167 | 0.231 | 0.314 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | **195558** | 0.099 | 0.159 | 0.240 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | **191316** | 0.103 | 0.156 | 0.227 |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | **85073** | 0.153 | 0.506 | 0.820 |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | **121979** | 0.154 | 0.215 | 0.285 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | **194470** | 0.097 | 0.165 | 0.282 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | **193523** | 0.094 | 0.174 | 0.310 |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | **592004** | 0.009 | 0.010 | 0.024 |

Throughput (SET/GET, workload 1): Nedis ≈ **3.5x SugarDB**. Hash workload: Nedis ≈ **2.3x SugarDB**. On workload 3 (ZSet), SugarDB's ZRANGE implements score-range rather than index-range semantics and returns empty sets under this workload, so its raw QPS is not comparable; Nedis and C Redis perform real index-range scans. SugarDB cannot run workloads 4–6 at all.

**Conclusion**: Against SugarDB, the most actively maintained pure-Go in-memory Redis alternative, Nedis delivers substantially higher throughput on the comparable workloads under identical suite conditions, and additionally supports Streams, Lua scripting, Bitmaps, and HyperLogLog, which SugarDB lacks entirely.
