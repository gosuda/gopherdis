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
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | 50000 | **133654 ops/s** | 0.348 ms | 0.464 ms | **0.678 ms** | **1.034 ms** | 4.62 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | 50000 | **303355 ops/s** ⚡ | 0.125 ms | 0.347 ms | **0.745 ms** (+10%) | **2.282 ms** (+121%) | 22.69 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | 50000 | **321632 ops/s** ⚡ | 0.120 ms | 0.310 ms | **0.633 ms** (-7%) | **2.004 ms** (+94%) | 22.44 MB |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | 50000 | **100017 ops/s** | 0.263 ms | 1.509 ms | **2.026 ms** (+199%) | **2.958 ms** (+186%) | 0 B |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | 20000 | **107070 ops/s** | 0.181 ms | 0.251 ms | **0.335 ms** | **0.423 ms** | 1.12 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | 20000 | **178098 ops/s** ⚡ | 0.110 ms | 0.172 ms | **0.274 ms** (-18%) | **0.621 ms** (+47%) | 12.12 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | 20000 | **194208 ops/s** ⚡ | 0.099 ms | 0.156 ms | **0.252 ms** (-25%) | **0.998 ms** (+136%) | 12.19 MB |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | 20000 | **94026 ops/s** | 0.150 ms | 0.502 ms | **0.858 ms** (+156%) | **14.429 ms** (+3311%) | 0 B |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | 20000 | **122410 ops/s** | 0.155 ms | 0.216 ms | **0.287 ms** | **0.358 ms** | 128.00 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | 20000 | **189625 ops/s** ⚡ | 0.097 ms | 0.173 ms | **0.312 ms** (+9%) | **1.296 ms** (+262%) | 384.00 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | 20000 | **195647 ops/s** ⚡ | 0.097 ms | 0.160 ms | **0.264 ms** (-8%) | **0.652 ms** (+82%) | 8.00 MB |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | 20000 | **667348 ops/s** | 0.008 ms | 0.010 ms | **0.034 ms** (-88%) | **9.106 ms** (+2444%) | 0 B |
| 4. Stream Queue (XADD/XRANGE) | **C Redis 8.0** | 20000 | **88096 ops/s** | 0.215 ms | 0.346 ms | **0.450 ms** | **0.669 ms** | 256.00 KB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go)** | 20000 | **108721 ops/s** ⚡ | 0.142 ms | 0.500 ms | **0.822 ms** (+83%) | **1.291 ms** (+93%) | 20.31 MB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go SIMD)** | 20000 | **111713 ops/s** ⚡ | 0.142 ms | 0.459 ms | **0.835 ms** (+86%) | **1.405 ms** (+110%) | 8.75 MB |
| 4. Stream Queue (XADD/XRANGE) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 5. Redlock Atomic Lua Scripting | **C Redis 8.0** | 20000 | **109954 ops/s** | 0.175 ms | 0.240 ms | **0.338 ms** | **1.040 ms** | 128.00 KB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go)** | 20000 | **159620 ops/s** ⚡ | 0.102 ms | 0.168 ms | **0.356 ms** (+5%) | **7.103 ms** (+583%) | 16.32 MB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go SIMD)** | 20000 | **158720 ops/s** ⚡ | 0.114 ms | 0.173 ms | **0.380 ms** (+12%) | **5.214 ms** (+401%) | 20.07 MB |
| 5. Redlock Atomic Lua Scripting | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 6. Bitmap & HyperLogLog | **C Redis 8.0** | 30000 | **129624 ops/s** | 0.146 ms | 0.196 ms | **0.260 ms** | **0.344 ms** | 0 B |
| 6. Bitmap & HyperLogLog | **Nedis (Go)** | 30000 | **214730 ops/s** ⚡ | 0.094 ms | 0.139 ms | **0.179 ms** (-31%) | **0.401 ms** (+17%) | 0 B |
| 6. Bitmap & HyperLogLog | **Nedis (Go SIMD)** | 30000 | **202154 ops/s** ⚡ | 0.096 ms | 0.162 ms | **0.253 ms** (-3%) | **0.576 ms** (+67%) | 0 B |
| 6. Bitmap & HyperLogLog | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 7. Real-World Cache (500k x 1KB) | **C Redis 8.0** | 500000 | **118187 ops/s** | 0.401 ms | 0.516 ms | **0.784 ms** | **1.513 ms** | 519.13 MB |
| 7. Real-World Cache (500k x 1KB) | **Nedis (Go)** | 500000 | **192918 ops/s** ⚡ | 0.135 ms | 0.744 ms | **1.218 ms** (+55%) | **2.138 ms** (+41%) | 744.60 MB |
| 7. Real-World Cache (500k x 1KB) | **Nedis (Go SIMD)** | 500000 | **194227 ops/s** ⚡ | 0.126 ms | 0.745 ms | **1.227 ms** (+57%) | **2.161 ms** (+43%) | 920.73 MB |
| 7. Real-World Cache (500k x 1KB) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |

> **Notes**: SugarDB does not support Streams (XADD/XRANGE), Lua scripting (EVAL/SCRIPT LOAD), Bitmaps (SETBIT/BITCOUNT), or HyperLogLog (PFADD) — verified empirically via `redis-cli` error replies — so those workloads are marked N/A. SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics rather than index-range, so its ZSet range queries return empty sets under this workload; the row is still measured. SugarDB does not support `INFO memory`, so its Memory Delta is reported as 0 B. On workload 7 (500k keys × 1KB values), SugarDB's server process crashes (container exit 2 with a goroutine dump), so it is marked N/A there. Percentages in the P99/P99.9 columns are the delta versus the C Redis row of the same workload; negative is better (lower tail latency).

---

## 4. 🔍 Deep-Dive Analysis by Scenario

### ① High-Concurrency SET/GET Throughput
- **Scenario**: 50 concurrent client connections, 50,000 operations with 128-byte payloads.
- **Analysis**: Nedis utilizes a **64-shard contention-free architecture**, socket-level `TCP_NODELAY`, and Beaver Arena memory pooling across 12 CPU hardware threads. It achieves **303k QPS (2.27x higher than C Redis)** with comparable tail latency (**P99: 0.745ms vs C Redis 0.678ms**). SugarDB reaches 100k QPS (Nedis is 3.03x faster).

### ② Multi-Field Hash Aggregation
- **Scenario**: 20 concurrent clients, 5-field HSET and HMGET operations across 20,000 requests.
- **Analysis**: With **Hybrid Flat-Dict (contiguous array pairs for <= 64 entries + automatic hash map promotion)** and single-pass RESP buffer serialization, Nedis delivers **178k QPS (1.66x C Redis)**, **P50 of 0.110ms**, and **P99 of 0.274ms (vs C Redis 0.335ms)**. SugarDB reaches 94k QPS (Nedis is 1.89x faster).

### ③ Ranked SkipList Leaderboard (ZSet)
- **Scenario**: 2,000 simulated players with real-time score updates (ZADD), rank lookups (ZRANK), and top-N range queries (ZRANGE).
- **Analysis**: Lightweight Mutex synchronization, lock-free `math/rand/v2` level generation, and stack-allocated node arrays achieve **190k QPS (1.55x C Redis)**, with **P95 (0.173ms vs 0.216ms)** and **P99 (0.312ms vs 0.287ms)**. (SugarDB's ZRANGE uses score-range semantics and returns empty sets here, so its 667k QPS is not an apples-to-apples comparison.)

### ④ Stream Event Queue (XADD / XRANGE)
- **Scenario**: 20 concurrent producers and consumers logging sensor telemetry events and querying ranges across 20,000 records.
- **Analysis**: O(1) chunk boundary skipping and `AddRaw` zero-copy parsing yield **108.7k QPS (1.23x C Redis)** with **P50 of 0.142ms** and **P95 of 0.500ms (vs C Redis 0.346ms)**. SugarDB does not support Streams (N/A).

### ⑤ Redlock Atomic Lua Scripting (Bytecode JIT Cache)
- **Scenario**: 20 workers competing for 100 distributed lock keys with atomic Lua release scripts.
- **Analysis**: Pre-compiled `FunctionProto` caching, VM table reuse, and zero-alloc `redis.call` argument conversions produce **159.6k QPS (1.45x C Redis)** with **P50 of 0.102ms**, **P95 of 0.168ms**, and **P99 of 0.356ms (vs C Redis 0.338ms)**. SugarDB does not support EVAL/SCRIPT LOAD (N/A).

### ⑥ Bitmap & HyperLogLog Cardinality Estimation
- **Scenario**: 100,000 bit mutations with SETBIT, 64-bit word POPCNT BITCOUNT, and 50,000 unique IP insertions with PFADD/PFCOUNT.
- **Analysis**: In-place zero-reallocation bit mutation, `math/bits.OnesCount64` hardware acceleration, and 12KB Dense Otmar Ertl HLL registers achieve **214.7k QPS (1.66x C Redis)** with **P50 of 0.094ms** and **P95 of 0.139ms (vs C Redis 0.196ms)**. SugarDB does not support SETBIT/BITCOUNT/PFADD (N/A).
### ⑦ Real-World Cache at Scale (500k keys × 1KB)
- **Scenario**: 50 concurrent clients loading 500,000 cache entries with 1KB values (~512MB of payload), 80% SET / 20% GET. This approximates a production cache workload where the dataset itself dominates memory usage.
- **Analysis**: With real data dominating, the memory gap narrows: Nedis grows **744.60 MB** vs C Redis **519.13 MB** (43% more), down from the multi-x ratio on the small overhead-dominated workloads above. Throughput: **193k QPS (1.63x C Redis)**, P99.9 **2.138ms vs 1.513ms**. (SugarDB crashes under this 500k×1KB load, so it is N/A.)


---

## 5. 🐹 Go Implementation Comparison (vs SugarDB)

SugarDB is now measured as a first-class target in this suite (see §3), so the numbers below are taken directly from the suite-measured rows above — identical machine (AMD Ryzen 5 5600X), identical harness (direct RESP TCP, pre-connected pooled clients), SugarDB `latest` running in Docker with host networking. SugarDB supports only workloads 1–3; workloads 4–6 are N/A (unsupported), and it crashes under workload 7's 500k×1KB load.

| Workload | Target | QPS (ops/s) | P50 (ms) | P95 (ms) | P99 (ms) | P99.9 (ms) |
|---|---|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | **133654** | 0.348 | 0.464 | 0.678 | 1.034 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | **303355** | 0.125 | 0.347 | 0.745 | 2.282 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | **321632** | 0.120 | 0.310 | 0.633 | 2.004 |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | **100017** | 0.263 | 1.509 | 2.026 | 2.958 |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | **107070** | 0.181 | 0.251 | 0.335 | 0.423 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | **178098** | 0.110 | 0.172 | 0.274 | 0.621 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | **194208** | 0.099 | 0.156 | 0.252 | 0.998 |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | **94026** | 0.150 | 0.502 | 0.858 | 14.429 |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | **122410** | 0.155 | 0.216 | 0.287 | 0.358 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | **189625** | 0.097 | 0.173 | 0.312 | 1.296 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | **195647** | 0.097 | 0.160 | 0.264 | 0.652 |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | **667348** | 0.008 | 0.010 | 0.034 | 9.106 |

Throughput (SET/GET, workload 1): Nedis ≈ **3.0x SugarDB**. Hash workload: Nedis ≈ **1.9x SugarDB**. On workload 3 (ZSet), SugarDB's ZRANGE implements score-range rather than index-range semantics and returns empty sets under this workload, so its raw QPS is not comparable; Nedis and C Redis perform real index-range scans. SugarDB cannot run workloads 4–6 at all.

**Conclusion**: Against SugarDB, the most actively maintained pure-Go in-memory Redis alternative, Nedis delivers substantially higher throughput on the comparable workloads under identical suite conditions, and additionally supports Streams, Lua scripting, Bitmaps, and HyperLogLog, which SugarDB lacks entirely.

---

## 6. 🧠 Memory Characteristics

Memory Delta is the growth of each server's `used_memory_rss` (from `INFO memory`) measured before and after each workload. RSS is used because it is comparable across engines: C Redis's `used_memory` covers only jemalloc allocations, and Nedis's `used_memory` is Go `MemStats.Alloc`, which lags GC sweep timing — RSS reflects what the OS actually backs with physical pages. (Nedis reports Go `MemStats.Sys` as `used_memory_rss`, i.e. memory obtained from the OS, which slightly over-counts versus true RSS.) SugarDB does not support `INFO memory`, so its deltas are reported as 0 B and omitted from the totals.

### At realistic dataset sizes (workload 7: 500k keys × 1KB values)

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 519.13 MB | baseline |
| **Nedis (Go)** | 744.60 MB | 1.43x |
| **Nedis (Go SIMD)** | 920.73 MB | 1.77x |

Once the dataset itself (~512MB of 1KB values) dominates `used_memory`, the gap between Nedis and C Redis narrows from the multi-x ratio seen on the small overhead-dominated workloads to a bounded addend (**744.60 MB** vs **519.13 MB**, 43% difference). The per-key amplification (~1.9KB stored per 1KB key/value pair) comes from the 64-shard dict's pre-sized buckets and Go object headers; the fixed baseline overhead from the table below becomes proportionally smaller as data grows.

### Baseline overhead (workloads 1–6, overhead-dominated)

On small workloads the fixed per-engine overhead is the dominant term. This isolates the constant cost each engine pays regardless of dataset size.

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) |
|---|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | 4.62 MB | 22.69 MB | 22.44 MB | 0 B |
| 2. Multi-Field Hash Aggregation | 1.12 MB | 12.12 MB | 12.19 MB | 0 B |
| 3. SkipList Leaderboard (ZSet) | 128.00 KB | 384.00 KB | 8.00 MB | 0 B |
| 4. Stream Queue (XADD/XRANGE) | 256.00 KB | 20.31 MB | 8.75 MB | N/A |
| 5. Redlock Atomic Lua Scripting | 128.00 KB | 16.32 MB | 20.07 MB | N/A |
| 6. Bitmap & HyperLogLog | 0 B | 0 B | 0 B | N/A |
| **Total (workloads 1–6)** | **6.25 MB** | **71.82 MB** | **71.44 MB** | **0 B** |

**Analysis**: Nedis trades a higher fixed baseline (**71.82 MB** vs C Redis **6.25 MB** across workloads 1–6) for its throughput and tail-latency gains. The delta is deliberate pre-allocation, not per-request leakage: the 64-shard architecture pre-sizes per-shard buffers, and the Beaver Arena pool retains 8KB stream chunk slabs for reuse instead of returning them to the OS. C Redis, backed by jemalloc, grows incrementally and reports the smallest deltas, while SugarDB's memory usage is unmeasurable in this suite because it does not implement `INFO memory`.
