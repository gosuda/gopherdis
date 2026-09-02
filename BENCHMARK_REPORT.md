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
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | 50000 | **123923 ops/s** | 0.387 ms | 0.494 ms | **0.730 ms** | **0.918 ms** | 4.90 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | 50000 | **313256 ops/s** ⚡ | 0.124 ms | 0.313 ms | **0.796 ms** | **2.064 ms** | 11.72 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | 50000 | **321851 ops/s** ⚡ | 0.127 ms | 0.309 ms | **0.588 ms** | **1.452 ms** | 12.40 MB |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | 50000 | **97855 ops/s** | 0.268 ms | 1.516 ms | **2.161 ms** | **4.705 ms** | 0 B |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | 20000 | **120440 ops/s** | 0.154 ms | 0.220 ms | **0.295 ms** | **0.393 ms** | 1.84 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | 20000 | **189238 ops/s** ⚡ | 0.102 ms | 0.159 ms | **0.285 ms** | **0.903 ms** | 13.60 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | 20000 | **196599 ops/s** ⚡ | 0.100 ms | 0.154 ms | **0.235 ms** | **0.572 ms** | 12.62 MB |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | 20000 | **97240 ops/s** | 0.146 ms | 0.480 ms | **0.736 ms** | **13.935 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | 20000 | **122047 ops/s** | 0.155 ms | 0.213 ms | **0.284 ms** | **0.357 ms** | 282.78 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | 20000 | **202417 ops/s** ⚡ | 0.091 ms | 0.162 ms | **0.271 ms** | **1.089 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | 20000 | **198623 ops/s** ⚡ | 0.093 ms | 0.160 ms | **0.254 ms** | **1.428 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | 20000 | **619799 ops/s** | 0.008 ms | 0.010 ms | **0.018 ms** | **8.531 ms** | 0 B |
| 4. Stream Queue (XADD/XRANGE) | **C Redis 8.0** | 20000 | **87330 ops/s** | 0.216 ms | 0.350 ms | **0.460 ms** | **0.861 ms** | 407.17 KB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go)** | 20000 | **108872 ops/s** ⚡ | 0.140 ms | 0.502 ms | **0.854 ms** | **1.323 ms** | 10.56 MB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go SIMD)** | 20000 | **111162 ops/s** ⚡ | 0.138 ms | 0.473 ms | **0.829 ms** | **1.735 ms** | 10.82 MB |
| 4. Stream Queue (XADD/XRANGE) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 5. Redlock Atomic Lua Scripting | **C Redis 8.0** | 20000 | **110752 ops/s** | 0.173 ms | 0.240 ms | **0.316 ms** | **0.379 ms** | 22.16 KB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go)** | 20000 | **78893 ops/s** ⚡ | 0.116 ms | 0.186 ms | **1.197 ms** | **6.002 ms** | 0 B |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go SIMD)** | 20000 | **136547 ops/s** ⚡ | 0.133 ms | 0.218 ms | **0.394 ms** | **5.186 ms** | 155.13 KB |
| 5. Redlock Atomic Lua Scripting | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 6. Bitmap & HyperLogLog | **C Redis 8.0** | 30000 | **133526 ops/s** | 0.139 ms | 0.191 ms | **0.259 ms** | **0.332 ms** | 112.98 KB |
| 6. Bitmap & HyperLogLog | **Nedis (Go)** | 30000 | **206107 ops/s** ⚡ | 0.093 ms | 0.154 ms | **0.223 ms** | **1.087 ms** | 9.96 MB |
| 6. Bitmap & HyperLogLog | **Nedis (Go SIMD)** | 30000 | **205275 ops/s** ⚡ | 0.098 ms | 0.147 ms | **0.193 ms** | **0.587 ms** | 9.91 MB |
| 6. Bitmap & HyperLogLog | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |

> **Notes**: SugarDB does not support Streams (XADD/XRANGE), Lua scripting (EVAL/SCRIPT LOAD), Bitmaps (SETBIT/BITCOUNT), or HyperLogLog (PFADD) — verified empirically via `redis-cli` error replies — so those workloads are marked N/A. SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics rather than index-range, so its ZSet range queries return empty sets under this workload; the row is still measured. SugarDB does not support `INFO memory`, so its Memory Delta is reported as 0 B.

---

## 4. 🔍 Deep-Dive Analysis by Scenario

### ① High-Concurrency SET/GET Throughput
- **Scenario**: 50 concurrent client connections, 50,000 operations with 128-byte payloads.
- **Analysis**: Nedis utilizes a **64-shard contention-free architecture**, socket-level `TCP_NODELAY`, and Beaver Arena memory pooling across 12 CPU hardware threads. It achieves **313k QPS (2.53x higher than C Redis)** with comparable tail latency (**P99: 0.796ms vs C Redis 0.730ms**). SugarDB reaches 98k QPS (Nedis is 3.20x faster).

### ② Multi-Field Hash Aggregation
- **Scenario**: 20 concurrent clients, 5-field HSET and HMGET operations across 20,000 requests.
- **Analysis**: With **Hybrid Flat-Dict (contiguous array pairs for <= 64 entries + automatic hash map promotion)** and single-pass RESP buffer serialization, Nedis delivers **189k QPS (1.57x C Redis)**, **P50 of 0.102ms**, and **P99 of 0.285ms (vs C Redis 0.295ms)**. SugarDB reaches 97k QPS (Nedis is 1.95x faster).

### ③ Ranked SkipList Leaderboard (ZSet)
- **Scenario**: 2,000 simulated players with real-time score updates (ZADD), rank lookups (ZRANK), and top-N range queries (ZRANGE).
- **Analysis**: Lightweight Mutex synchronization, lock-free `math/rand/v2` level generation, and stack-allocated node arrays achieve **202k QPS (1.66x C Redis)**, with **P95 (0.162ms vs 0.213ms)** and **P99 (0.271ms vs 0.284ms)**. (SugarDB's ZRANGE uses score-range semantics and returns empty sets here, so its 620k QPS is not an apples-to-apples comparison.)

### ④ Stream Event Queue (XADD / XRANGE)
- **Scenario**: 20 concurrent producers and consumers logging sensor telemetry events and querying ranges across 20,000 records.
- **Analysis**: O(1) chunk boundary skipping and `AddRaw` zero-copy parsing yield **108.9k QPS (1.25x C Redis)** with **P50 of 0.140ms** and **P95 of 0.502ms (vs C Redis 0.350ms)**. SugarDB does not support Streams (N/A).

### ⑤ Redlock Atomic Lua Scripting (Bytecode JIT Cache)
- **Scenario**: 20 workers competing for 100 distributed lock keys with atomic Lua release scripts.
- **Analysis**: Pre-compiled `FunctionProto` caching, VM table reuse, and zero-alloc `redis.call` argument conversions produce **78.9k QPS (0.71x C Redis)** with **P50 of 0.116ms**, **P95 of 0.186ms**, and **P99 of 1.197ms (vs C Redis 0.316ms)**. SugarDB does not support EVAL/SCRIPT LOAD (N/A).

### ⑥ Bitmap & HyperLogLog Cardinality Estimation
- **Scenario**: 100,000 bit mutations with SETBIT, 64-bit word POPCNT BITCOUNT, and 50,000 unique IP insertions with PFADD/PFCOUNT.
- **Analysis**: In-place zero-reallocation bit mutation, `math/bits.OnesCount64` hardware acceleration, and 12KB Dense Otmar Ertl HLL registers achieve **206.1k QPS (1.54x C Redis)** with **P50 of 0.093ms** and **P95 of 0.154ms (vs C Redis 0.191ms)**. SugarDB does not support SETBIT/BITCOUNT/PFADD (N/A).

---

## 5. 🐹 Go Implementation Comparison (vs SugarDB)

SugarDB is now measured as a first-class target in this suite (see §3), so the numbers below are taken directly from the suite-measured rows above — identical machine (AMD Ryzen 5 5600X), identical harness (direct RESP TCP, pre-connected pooled clients), SugarDB `latest` running in Docker with host networking. SugarDB supports only workloads 1–3; workloads 4–6 are N/A (unsupported).

| Workload | Target | QPS (ops/s) | P50 (ms) | P95 (ms) | P99 (ms) | P99.9 (ms) |
|---|---|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | **123923** | 0.387 | 0.494 | 0.730 | 0.918 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | **313256** | 0.124 | 0.313 | 0.796 | 2.064 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | **321851** | 0.127 | 0.309 | 0.588 | 1.452 |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | **97855** | 0.268 | 1.516 | 2.161 | 4.705 |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | **120440** | 0.154 | 0.220 | 0.295 | 0.393 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | **189238** | 0.102 | 0.159 | 0.285 | 0.903 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | **196599** | 0.100 | 0.154 | 0.235 | 0.572 |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | **97240** | 0.146 | 0.480 | 0.736 | 13.935 |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | **122047** | 0.155 | 0.213 | 0.284 | 0.357 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | **202417** | 0.091 | 0.162 | 0.271 | 1.089 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | **198623** | 0.093 | 0.160 | 0.254 | 1.428 |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | **619799** | 0.008 | 0.010 | 0.018 | 8.531 |

Throughput (SET/GET, workload 1): Nedis ≈ **3.2x SugarDB**. Hash workload: Nedis ≈ **1.9x SugarDB**. On workload 3 (ZSet), SugarDB's ZRANGE implements score-range rather than index-range semantics and returns empty sets under this workload, so its raw QPS is not comparable; Nedis and C Redis perform real index-range scans. SugarDB cannot run workloads 4–6 at all.

**Conclusion**: Against SugarDB, the most actively maintained pure-Go in-memory Redis alternative, Nedis delivers substantially higher throughput on the comparable workloads under identical suite conditions, and additionally supports Streams, Lua scripting, Bitmaps, and HyperLogLog, which SugarDB lacks entirely.

---

## 6. 🧠 Memory Characteristics

Memory Delta is the growth of each server's `used_memory` (from `INFO memory`) measured before and after each workload. It captures how much additional RAM each engine allocates to serve the same operations — reflecting allocator behavior, arena pooling, and per-shard buffer pre-allocation. SugarDB does not support `INFO memory`, so its deltas are reported as 0 B and omitted from the totals.

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) |
|---|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | 4.90 MB | 11.72 MB | 12.40 MB | 0 B |
| 2. Multi-Field Hash Aggregation | 1.84 MB | 13.60 MB | 12.62 MB | 0 B |
| 3. SkipList Leaderboard (ZSet) | 282.78 KB | 0 B | 0 B | 0 B |
| 4. Stream Queue (XADD/XRANGE) | 407.17 KB | 10.56 MB | 10.82 MB | N/A |
| 5. Redlock Atomic Lua Scripting | 22.16 KB | 0 B | 155.13 KB | N/A |
| 6. Bitmap & HyperLogLog | 112.98 KB | 9.96 MB | 9.91 MB | N/A |
| **Total (6 workloads)** | **7.55 MB** | **45.84 MB** | **45.90 MB** | **0 B** |

**Analysis**: Nedis trades a higher baseline memory footprint (total delta **45.84 MB** vs C Redis **7.55 MB** across all six workloads) for its throughput and tail-latency gains. The delta is dominated by deliberate pre-allocation rather than per-request leakage: the 64-shard architecture pre-sizes per-shard buffers, and the Beaver Arena pool retains 8KB stream chunk slabs for reuse instead of returning them to the OS. C Redis, backed by jemalloc, grows incrementally and reports the smallest deltas, while SugarDB's memory usage is unmeasurable in this suite because it does not implement `INFO memory`.
