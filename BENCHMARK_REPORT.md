# 📊 C Redis 8.0 vs Nedis (Pure Go) vs SugarDB (Pure Go) 7-Dimensional Benchmark Analysis Report

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
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | 50000 | **129591 ops/s** | 0.366 ms | 0.465 ms | **0.721 ms** | **1.198 ms** | 4.50 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | 50000 | **313822 ops/s** ⚡ | 0.135 ms | 0.306 ms | **0.547 ms** | **1.559 ms** | 22.38 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | 50000 | **319249 ops/s** ⚡ | 0.129 ms | 0.304 ms | **0.635 ms** | **1.354 ms** | 22.62 MB |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | 50000 | **105872 ops/s** | 0.248 ms | 1.481 ms | **2.038 ms** | **2.930 ms** | 0 B |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | 20000 | **115000 ops/s** | 0.163 ms | 0.234 ms | **0.308 ms** | **0.433 ms** | 1.62 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | 20000 | **189069 ops/s** ⚡ | 0.104 ms | 0.161 ms | **0.238 ms** | **0.699 ms** | 12.19 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | 20000 | **189965 ops/s** ⚡ | 0.102 ms | 0.160 ms | **0.258 ms** | **0.553 ms** | 12.38 MB |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | 20000 | **93974 ops/s** | 0.147 ms | 0.513 ms | **0.784 ms** | **1.330 ms** | 0 B |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | 20000 | **118036 ops/s** | 0.156 ms | 0.235 ms | **0.318 ms** | **0.481 ms** | 128.00 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | 20000 | **169995 ops/s** ⚡ | 0.112 ms | 0.184 ms | **0.316 ms** | **1.106 ms** | 256.00 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | 20000 | **198830 ops/s** ⚡ | 0.095 ms | 0.163 ms | **0.278 ms** | **0.614 ms** | 64.00 KB |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | 20000 | **637205 ops/s** | 0.009 ms | 0.011 ms | **0.054 ms** | **7.851 ms** | 0 B |
| 4. Stream Queue (XADD/XRANGE) | **C Redis 8.0** | 20000 | **87646 ops/s** | 0.218 ms | 0.346 ms | **0.445 ms** | **0.559 ms** | 384.00 KB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go)** | 20000 | **111670 ops/s** ⚡ | 0.137 ms | 0.473 ms | **0.832 ms** | **1.744 ms** | 20.94 MB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go SIMD)** | 20000 | **111107 ops/s** ⚡ | 0.144 ms | 0.436 ms | **0.798 ms** | **1.433 ms** | 20.44 MB |
| 4. Stream Queue (XADD/XRANGE) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 5. Redlock Atomic Lua Scripting | **C Redis 8.0** | 20000 | **111578 ops/s** | 0.170 ms | 0.242 ms | **0.311 ms** | **0.378 ms** | 0 B |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go)** | 20000 | **156551 ops/s** ⚡ | 0.115 ms | 0.180 ms | **0.306 ms** | **6.151 ms** | 16.07 MB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go SIMD)** | 20000 | **147882 ops/s** ⚡ | 0.119 ms | 0.191 ms | **0.406 ms** | **5.754 ms** | 16.07 MB |
| 5. Redlock Atomic Lua Scripting | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 6. Bitmap & HyperLogLog | **C Redis 8.0** | 30000 | **131605 ops/s** | 0.142 ms | 0.198 ms | **0.258 ms** | **0.338 ms** | 128.00 KB |
| 6. Bitmap & HyperLogLog | **Nedis (Go)** | 30000 | **212911 ops/s** ⚡ | 0.093 ms | 0.141 ms | **0.190 ms** | **0.713 ms** | 0 B |
| 6. Bitmap & HyperLogLog | **Nedis (Go SIMD)** | 30000 | **210141 ops/s** ⚡ | 0.095 ms | 0.143 ms | **0.181 ms** | **0.396 ms** | 0 B |
| 6. Bitmap & HyperLogLog | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 7. Real-World Cache (500k x 1KB) | **C Redis 8.0** | 500000 | **117916 ops/s** | 0.403 ms | 0.516 ms | **0.790 ms** | **1.538 ms** | 523.18 MB |
| 7. Real-World Cache (500k x 1KB) | **Nedis (Go)** | 500000 | **193763 ops/s** ⚡ | 0.128 ms | 0.744 ms | **1.211 ms** | **2.417 ms** | 814.17 MB |
| 7. Real-World Cache (500k x 1KB) | **Nedis (Go SIMD)** | 500000 | **193823 ops/s** ⚡ | 0.129 ms | 0.745 ms | **1.203 ms** | **2.103 ms** | 863.48 MB |
| 7. Real-World Cache (500k x 1KB) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |

> **Notes**: SugarDB does not support Streams (XADD/XRANGE), Lua scripting (EVAL/SCRIPT LOAD), Bitmaps (SETBIT/BITCOUNT), or HyperLogLog (PFADD) — verified empirically via `redis-cli` error replies — so those workloads are marked N/A. SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics rather than index-range, so its ZSet range queries return empty sets under this workload; the row is still measured. SugarDB does not support `INFO memory`, so its Memory Delta is reported as 0 B. On workload 7 (500k keys × 1KB values), SugarDB's server process crashes (container exit 2 with a goroutine dump), so it is marked N/A there.

---

## 4. 🔍 Deep-Dive Analysis by Scenario

### ① High-Concurrency SET/GET Throughput
- **Scenario**: 50 concurrent client connections, 50,000 operations with 128-byte payloads.
- **Analysis**: Nedis utilizes a **64-shard contention-free architecture**, socket-level `TCP_NODELAY`, and Beaver Arena memory pooling across 12 CPU hardware threads. It achieves **314k QPS (2.42x higher than C Redis)** with comparable tail latency (**P99: 0.547ms vs C Redis 0.721ms**). SugarDB reaches 106k QPS (Nedis is 2.96x faster).

### ② Multi-Field Hash Aggregation
- **Scenario**: 20 concurrent clients, 5-field HSET and HMGET operations across 20,000 requests.
- **Analysis**: With **Hybrid Flat-Dict (contiguous array pairs for <= 64 entries + automatic hash map promotion)** and single-pass RESP buffer serialization, Nedis delivers **189k QPS (1.64x C Redis)**, **P50 of 0.104ms**, and **P99 of 0.238ms (vs C Redis 0.308ms)**. SugarDB reaches 94k QPS (Nedis is 2.01x faster).

### ③ Ranked SkipList Leaderboard (ZSet)
- **Scenario**: 2,000 simulated players with real-time score updates (ZADD), rank lookups (ZRANK), and top-N range queries (ZRANGE).
- **Analysis**: Lightweight Mutex synchronization, lock-free `math/rand/v2` level generation, and stack-allocated node arrays achieve **170k QPS (1.44x C Redis)**, with **P95 (0.184ms vs 0.235ms)** and **P99 (0.316ms vs 0.318ms)**. (SugarDB's ZRANGE uses score-range semantics and returns empty sets here, so its 637k QPS is not an apples-to-apples comparison.)

### ④ Stream Event Queue (XADD / XRANGE)
- **Scenario**: 20 concurrent producers and consumers logging sensor telemetry events and querying ranges across 20,000 records.
- **Analysis**: O(1) chunk boundary skipping and `AddRaw` zero-copy parsing yield **111.7k QPS (1.27x C Redis)** with **P50 of 0.137ms** and **P95 of 0.473ms (vs C Redis 0.346ms)**. SugarDB does not support Streams (N/A).

### ⑤ Redlock Atomic Lua Scripting (Bytecode JIT Cache)
- **Scenario**: 20 workers competing for 100 distributed lock keys with atomic Lua release scripts.
- **Analysis**: Pre-compiled `FunctionProto` caching, VM table reuse, and zero-alloc `redis.call` argument conversions produce **156.6k QPS (1.40x C Redis)** with **P50 of 0.115ms**, **P95 of 0.180ms**, and **P99 of 0.306ms (vs C Redis 0.311ms)**. SugarDB does not support EVAL/SCRIPT LOAD (N/A).

### ⑥ Bitmap & HyperLogLog Cardinality Estimation
- **Scenario**: 100,000 bit mutations with SETBIT, 64-bit word POPCNT BITCOUNT, and 50,000 unique IP insertions with PFADD/PFCOUNT.
- **Analysis**: In-place zero-reallocation bit mutation, `math/bits.OnesCount64` hardware acceleration, and 12KB Dense Otmar Ertl HLL registers achieve **212.9k QPS (1.62x C Redis)** with **P50 of 0.093ms** and **P95 of 0.141ms (vs C Redis 0.198ms)**. SugarDB does not support SETBIT/BITCOUNT/PFADD (N/A).
### ⑦ Real-World Cache at Scale (500k keys × 1KB)
- **Scenario**: 50 concurrent clients loading 500,000 cache entries with 1KB values (~512MB of payload), 80% SET / 20% GET. This approximates a production cache workload where the dataset itself dominates memory usage.
- **Analysis**: With real data dominating, the memory gap narrows: Nedis grows **814.17 MB** vs C Redis **523.18 MB** (56% more), down from the multi-x ratio on the small overhead-dominated workloads above. Throughput: **194k QPS (1.64x C Redis)**, P99.9 **2.417ms vs 1.538ms**. (SugarDB crashes under this 500k×1KB load, so it is N/A.)


---

## 5. 🐹 Go Implementation Comparison (vs SugarDB)

SugarDB is now measured as a first-class target in this suite (see §3), so the numbers below are taken directly from the suite-measured rows above — identical machine (AMD Ryzen 5 5600X), identical harness (direct RESP TCP, pre-connected pooled clients), SugarDB `latest` running in Docker with host networking. SugarDB supports only workloads 1–3; workloads 4–6 are N/A (unsupported), and it crashes under workload 7's 500k×1KB load.

| Workload | Target | QPS (ops/s) | P50 (ms) | P95 (ms) | P99 (ms) | P99.9 (ms) |
|---|---|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | **129591** | 0.366 | 0.465 | 0.721 | 1.198 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | **313822** | 0.135 | 0.306 | 0.547 | 1.559 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | **319249** | 0.129 | 0.304 | 0.635 | 1.354 |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | **105872** | 0.248 | 1.481 | 2.038 | 2.930 |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | **115000** | 0.163 | 0.234 | 0.308 | 0.433 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | **189069** | 0.104 | 0.161 | 0.238 | 0.699 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | **189965** | 0.102 | 0.160 | 0.258 | 0.553 |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | **93974** | 0.147 | 0.513 | 0.784 | 1.330 |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | **118036** | 0.156 | 0.235 | 0.318 | 0.481 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | **169995** | 0.112 | 0.184 | 0.316 | 1.106 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | **198830** | 0.095 | 0.163 | 0.278 | 0.614 |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | **637205** | 0.009 | 0.011 | 0.054 | 7.851 |

Throughput (SET/GET, workload 1): Nedis ≈ **3.0x SugarDB**. Hash workload: Nedis ≈ **2.0x SugarDB**. On workload 3 (ZSet), SugarDB's ZRANGE implements score-range rather than index-range semantics and returns empty sets under this workload, so its raw QPS is not comparable; Nedis and C Redis perform real index-range scans. SugarDB cannot run workloads 4–6 at all.

**Conclusion**: Against SugarDB, the most actively maintained pure-Go in-memory Redis alternative, Nedis delivers substantially higher throughput on the comparable workloads under identical suite conditions, and additionally supports Streams, Lua scripting, Bitmaps, and HyperLogLog, which SugarDB lacks entirely.

---

## 6. 🧠 Memory Characteristics

Memory Delta is the growth of each server's `used_memory_rss` (from `INFO memory`) measured before and after each workload. RSS is used because it is comparable across engines: C Redis's `used_memory` covers only jemalloc allocations, and Nedis's `used_memory` is Go `MemStats.Alloc`, which lags GC sweep timing — RSS reflects what the OS actually backs with physical pages. (Nedis reports Go `MemStats.Sys` as `used_memory_rss`, i.e. memory obtained from the OS, which slightly over-counts versus true RSS.) SugarDB does not support `INFO memory`, so its deltas are reported as 0 B and omitted from the totals.

### At realistic dataset sizes (workload 7: 500k keys × 1KB values)

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 523.18 MB | baseline |
| **Nedis (Go)** | 814.17 MB | 1.56x |
| **Nedis (Go SIMD)** | 863.48 MB | 1.65x |

Once the dataset itself (~512MB of 1KB values) dominates `used_memory`, the gap between Nedis and C Redis narrows from the multi-x ratio seen on the small overhead-dominated workloads to a bounded addend (**814.17 MB** vs **523.18 MB**, 56% difference). The per-key amplification (~1.9KB stored per 1KB key/value pair) comes from the 64-shard dict's pre-sized buckets and Go object headers; the fixed baseline overhead from the table below becomes proportionally smaller as data grows.

### Baseline overhead (workloads 1–6, overhead-dominated)

On small workloads the fixed per-engine overhead is the dominant term. This isolates the constant cost each engine pays regardless of dataset size.

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) |
|---|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | 4.50 MB | 22.38 MB | 22.62 MB | 0 B |
| 2. Multi-Field Hash Aggregation | 1.62 MB | 12.19 MB | 12.38 MB | 0 B |
| 3. SkipList Leaderboard (ZSet) | 128.00 KB | 256.00 KB | 64.00 KB | 0 B |
| 4. Stream Queue (XADD/XRANGE) | 384.00 KB | 20.94 MB | 20.44 MB | N/A |
| 5. Redlock Atomic Lua Scripting | 0 B | 16.07 MB | 16.07 MB | N/A |
| 6. Bitmap & HyperLogLog | 128.00 KB | 0 B | 0 B | N/A |
| **Total (workloads 1–6)** | **6.75 MB** | **71.82 MB** | **71.57 MB** | **0 B** |

**Analysis**: Nedis trades a higher fixed baseline (**71.82 MB** vs C Redis **6.75 MB** across workloads 1–6) for its throughput and tail-latency gains. The delta is deliberate pre-allocation, not per-request leakage: the 64-shard architecture pre-sizes per-shard buffers, and the Beaver Arena pool retains 8KB stream chunk slabs for reuse instead of returning them to the OS. C Redis, backed by jemalloc, grows incrementally and reports the smallest deltas, while SugarDB's memory usage is unmeasurable in this suite because it does not implement `INFO memory`.
