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

| Workload Scenario | Target Engine | Total Ops | Throughput (QPS) | P50 Latency (ms) | P95 Latency (ms) | P99 Latency (ms) | P99.9 Latency (ms) | Memory Delta |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | 50000 | **133211 ops/s** | 0.352 ms | 0.457 ms | **0.688 ms** | **0.897 ms** | 4.90 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | 50000 | **296553 ops/s** ⚡ | 0.133 ms | 0.338 ms | **0.735 ms** | **1.931 ms** | 13.36 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | 50000 | **312358 ops/s** ⚡ | 0.138 ms | 0.301 ms | **0.542 ms** | **1.279 ms** | 13.19 MB |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | 50000 | **97679 ops/s** | 0.271 ms | 1.531 ms | **2.084 ms** | **3.740 ms** | 0 B |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | 20000 | **116930 ops/s** | 0.160 ms | 0.226 ms | **0.312 ms** | **0.884 ms** | 1.84 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | 20000 | **152937 ops/s** ⚡ | 0.117 ms | 0.236 ms | **0.483 ms** | **1.646 ms** | 12.86 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | 20000 | **189946 ops/s** ⚡ | 0.102 ms | 0.161 ms | **0.246 ms** | **0.547 ms** | 13.08 MB |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | 20000 | **94610 ops/s** | 0.149 ms | 0.485 ms | **0.762 ms** | **14.313 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | 20000 | **123174 ops/s** | 0.151 ms | 0.210 ms | **0.283 ms** | **0.358 ms** | 281.88 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | 20000 | **167198 ops/s** ⚡ | 0.108 ms | 0.198 ms | **0.484 ms** | **1.212 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | 20000 | **165103 ops/s** ⚡ | 0.112 ms | 0.200 ms | **0.349 ms** | **1.261 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | 20000 | **656722 ops/s** | 0.009 ms | 0.011 ms | **0.023 ms** | **9.383 ms** | 0 B |
| 4. Stream Queue (XADD/XRANGE) | **C Redis 8.0** | 20000 | **82268 ops/s** | 0.225 ms | 0.385 ms | **0.576 ms** | **1.346 ms** | 407.14 KB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go)** | 20000 | **110031 ops/s** ⚡ | 0.141 ms | 0.481 ms | **0.774 ms** | **1.881 ms** | 12.48 MB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go SIMD)** | 20000 | **110977 ops/s** ⚡ | 0.139 ms | 0.464 ms | **0.838 ms** | **1.645 ms** | 10.80 MB |
| 4. Stream Queue (XADD/XRANGE) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 5. Redlock Atomic Lua Scripting | **C Redis 8.0** | 20000 | **112896 ops/s** | 0.167 ms | 0.232 ms | **0.313 ms** | **0.378 ms** | 20.15 KB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go)** | 20000 | **155053 ops/s** ⚡ | 0.113 ms | 0.171 ms | **0.320 ms** | **5.785 ms** | 1.39 MB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go SIMD)** | 20000 | **154827 ops/s** ⚡ | 0.108 ms | 0.182 ms | **0.465 ms** | **5.943 ms** | 0 B |
| 5. Redlock Atomic Lua Scripting | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 6. Bitmap & HyperLogLog | **C Redis 8.0** | 30000 | **128573 ops/s** | 0.148 ms | 0.198 ms | **0.267 ms** | **0.341 ms** | 116.98 KB |
| 6. Bitmap & HyperLogLog | **Nedis (Go)** | 30000 | **213633 ops/s** ⚡ | 0.094 ms | 0.138 ms | **0.171 ms** | **0.370 ms** | 9.94 MB |
| 6. Bitmap & HyperLogLog | **Nedis (Go SIMD)** | 30000 | **204987 ops/s** ⚡ | 0.092 ms | 0.158 ms | **0.244 ms** | **1.204 ms** | 9.89 MB |
| 6. Bitmap & HyperLogLog | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |

> **Notes**: SugarDB does not support Streams (XADD/XRANGE), Lua scripting (EVAL/SCRIPT LOAD), Bitmaps (SETBIT/BITCOUNT), or HyperLogLog (PFADD) — verified empirically via `redis-cli` error replies — so those workloads are marked N/A. SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics rather than index-range, so its ZSet range queries return empty sets under this workload; the row is still measured. SugarDB does not support `INFO memory`, so its Memory Delta is reported as 0 B.

---

## 4. 🔍 Deep-Dive Analysis by Scenario

### ① High-Concurrency SET/GET Throughput
- **Scenario**: 50 concurrent client connections, 50,000 operations with 128-byte payloads.
- **Analysis**: Nedis utilizes a **64-shard contention-free architecture**, socket-level `TCP_NODELAY`, and Beaver Arena memory pooling across 12 CPU hardware threads. It achieves **297k QPS (2.23x higher than C Redis)** with comparable tail latency (**P99: 0.735ms vs C Redis 0.688ms**). SugarDB reaches 98k QPS (Nedis is 3.04x faster).

### ② Multi-Field Hash Aggregation
- **Scenario**: 20 concurrent clients, 5-field HSET and HMGET operations across 20,000 requests.
- **Analysis**: With **Hybrid Flat-Dict (contiguous array pairs for <= 64 entries + automatic hash map promotion)** and single-pass RESP buffer serialization, Nedis delivers **153k QPS (1.31x C Redis)**, **P50 of 0.117ms**, and **P99 of 0.483ms (vs C Redis 0.312ms)**. SugarDB reaches 95k QPS (Nedis is 1.62x faster).

### ③ Ranked SkipList Leaderboard (ZSet)
- **Scenario**: 2,000 simulated players with real-time score updates (ZADD), rank lookups (ZRANK), and top-N range queries (ZRANGE).
- **Analysis**: Lightweight Mutex synchronization, lock-free `math/rand/v2` level generation, and stack-allocated node arrays achieve **167k QPS (1.36x C Redis)**, with **P95 (0.198ms vs 0.210ms)** and **P99 (0.484ms vs 0.283ms)**. (SugarDB's ZRANGE uses score-range semantics and returns empty sets here, so its 657k QPS is not an apples-to-apples comparison.)

### ④ Stream Event Queue (XADD / XRANGE)
- **Scenario**: 20 concurrent producers and consumers logging sensor telemetry events and querying ranges across 20,000 records.
- **Analysis**: O(1) chunk boundary skipping and `AddRaw` zero-copy parsing yield **110.0k QPS (1.34x C Redis)** with **P50 of 0.141ms** and **P95 of 0.481ms (vs C Redis 0.385ms)**. SugarDB does not support Streams (N/A).

### ⑤ Redlock Atomic Lua Scripting (Bytecode JIT Cache)
- **Scenario**: 20 workers competing for 100 distributed lock keys with atomic Lua release scripts.
- **Analysis**: Pre-compiled `FunctionProto` caching, VM table reuse, and zero-alloc `redis.call` argument conversions produce **155.1k QPS (1.37x C Redis)** with **P50 of 0.113ms**, **P95 of 0.171ms**, and **P99 of 0.320ms (vs C Redis 0.313ms)**. SugarDB does not support EVAL/SCRIPT LOAD (N/A).

### ⑥ Bitmap & HyperLogLog Cardinality Estimation
- **Scenario**: 100,000 bit mutations with SETBIT, 64-bit word POPCNT BITCOUNT, and 50,000 unique IP insertions with PFADD/PFCOUNT.
- **Analysis**: In-place zero-reallocation bit mutation, `math/bits.OnesCount64` hardware acceleration, and 12KB Dense Otmar Ertl HLL registers achieve **213.6k QPS (1.66x C Redis)** with **P50 of 0.094ms** and **P95 of 0.138ms (vs C Redis 0.198ms)**. SugarDB does not support SETBIT/BITCOUNT/PFADD (N/A).

---

## 5. 🐹 Go Implementation Comparison (vs SugarDB)

SugarDB is now measured as a first-class target in this suite (see §3), so the numbers below are taken directly from the suite-measured rows above — identical machine (AMD Ryzen 5 5600X), identical harness (direct RESP TCP, pre-connected pooled clients), SugarDB `latest` running in Docker with host networking. SugarDB supports only workloads 1–3; workloads 4–6 are N/A (unsupported).

| Workload | Target | QPS (ops/s) | P50 (ms) | P95 (ms) | P99 (ms) | P99.9 (ms) |
|---|---|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | **133211** | 0.352 | 0.457 | 0.688 | 0.897 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | **296553** | 0.133 | 0.338 | 0.735 | 1.931 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | **312358** | 0.138 | 0.301 | 0.542 | 1.279 |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | **97679** | 0.271 | 1.531 | 2.084 | 3.740 |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | **116930** | 0.160 | 0.226 | 0.312 | 0.884 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | **152937** | 0.117 | 0.236 | 0.483 | 1.646 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | **189946** | 0.102 | 0.161 | 0.246 | 0.547 |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | **94610** | 0.149 | 0.485 | 0.762 | 14.313 |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | **123174** | 0.151 | 0.210 | 0.283 | 0.358 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | **167198** | 0.108 | 0.198 | 0.484 | 1.212 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | **165103** | 0.112 | 0.200 | 0.349 | 1.261 |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | **656722** | 0.009 | 0.011 | 0.023 | 9.383 |

Throughput (SET/GET, workload 1): Nedis ≈ **3.0x SugarDB**. Hash workload: Nedis ≈ **1.6x SugarDB**. On workload 3 (ZSet), SugarDB's ZRANGE implements score-range rather than index-range semantics and returns empty sets under this workload, so its raw QPS is not comparable; Nedis and C Redis perform real index-range scans. SugarDB cannot run workloads 4–6 at all.

**Conclusion**: Against SugarDB, the most actively maintained pure-Go in-memory Redis alternative, Nedis delivers substantially higher throughput on the comparable workloads under identical suite conditions, and additionally supports Streams, Lua scripting, Bitmaps, and HyperLogLog, which SugarDB lacks entirely.
