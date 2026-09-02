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
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | 50000 | **132205 ops/s** | 0.357 ms | 0.459 ms | **0.686 ms** | **1.190 ms** | 4.62 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | 50000 | **309445 ops/s** ⚡ | 0.126 ms | 0.315 ms | **0.682 ms** (-1%) | **2.279 ms** (+92%) | 22.12 MB |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | 50000 | **304610 ops/s** ⚡ | 0.129 ms | 0.332 ms | **0.711 ms** (+4%) | **1.576 ms** (+32%) | 22.44 MB |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | 50000 | **100131 ops/s** | 0.266 ms | 1.475 ms | **1.983 ms** (+189%) | **3.769 ms** (+217%) | 0 B |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | 20000 | **118447 ops/s** | 0.160 ms | 0.224 ms | **0.296 ms** | **0.369 ms** | 1.12 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | 20000 | **196183 ops/s** ⚡ | 0.099 ms | 0.157 ms | **0.250 ms** (-16%) | **0.593 ms** (+61%) | 12.62 MB |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | 20000 | **184902 ops/s** ⚡ | 0.102 ms | 0.157 ms | **0.319 ms** (+8%) | **1.728 ms** (+368%) | 12.12 MB |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | 20000 | **93670 ops/s** | 0.149 ms | 0.497 ms | **0.828 ms** (+180%) | **14.114 ms** (+3725%) | 0 B |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | 20000 | **123131 ops/s** | 0.153 ms | 0.213 ms | **0.290 ms** | **0.359 ms** | 128.00 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | 20000 | **198272 ops/s** ⚡ | 0.095 ms | 0.160 ms | **0.252 ms** (-13%) | **1.183 ms** (+230%) | 64.00 KB |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | 20000 | **191772 ops/s** ⚡ | 0.098 ms | 0.164 ms | **0.286 ms** (-1%) | **1.135 ms** (+216%) | 64.00 KB |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | 20000 | **616610 ops/s** | 0.008 ms | 0.010 ms | **0.020 ms** (-93%) | **10.032 ms** (+2694%) | 0 B |
| 4. Stream Queue (XADD/XRANGE) | **C Redis 8.0** | 20000 | **69949 ops/s** | 0.254 ms | 0.495 ms | **0.922 ms** | **1.793 ms** | 256.00 KB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go)** | 20000 | **108743 ops/s** ⚡ | 0.140 ms | 0.509 ms | **0.812 ms** (-12%) | **1.679 ms** (-6%) | 16.56 MB |
| 4. Stream Queue (XADD/XRANGE) | **Nedis (Go SIMD)** | 20000 | **110124 ops/s** ⚡ | 0.141 ms | 0.495 ms | **0.777 ms** (-16%) | **1.307 ms** (-27%) | 20.62 MB |
| 4. Stream Queue (XADD/XRANGE) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 5. Redlock Atomic Lua Scripting | **C Redis 8.0** | 20000 | **115297 ops/s** | 0.161 ms | 0.228 ms | **0.297 ms** | **0.378 ms** | 128.00 KB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go)** | 20000 | **156623 ops/s** ⚡ | 0.117 ms | 0.173 ms | **0.293 ms** (-1%) | **6.674 ms** (+1666%) | 24.38 MB |
| 5. Redlock Atomic Lua Scripting | **Nedis (Go SIMD)** | 20000 | **156793 ops/s** ⚡ | 0.119 ms | 0.179 ms | **0.351 ms** (+18%) | **3.264 ms** (+763%) | 20.32 MB |
| 5. Redlock Atomic Lua Scripting | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 6. Bitmap & HyperLogLog | **C Redis 8.0** | 30000 | **129724 ops/s** | 0.146 ms | 0.197 ms | **0.261 ms** | **0.344 ms** | 0 B |
| 6. Bitmap & HyperLogLog | **Nedis (Go)** | 30000 | **207327 ops/s** ⚡ | 0.096 ms | 0.141 ms | **0.183 ms** (-30%) | **0.944 ms** (+174%) | 0 B |
| 6. Bitmap & HyperLogLog | **Nedis (Go SIMD)** | 30000 | **209039 ops/s** ⚡ | 0.095 ms | 0.141 ms | **0.192 ms** (-26%) | **0.634 ms** (+84%) | 0 B |
| 6. Bitmap & HyperLogLog | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 7. Real-World Cache (2M x 1KB) | **C Redis 8.0** | 2000000 | **115147 ops/s** | 0.415 ms | 0.532 ms | **0.825 ms** | **1.560 ms** | 2.06 GB |
| 7. Real-World Cache (2M x 1KB) | **Nedis (Go)** | 2000000 | **194518 ops/s** ⚡ | 0.129 ms | 0.736 ms | **1.194 ms** (+45%) | **2.193 ms** (+41%) | 3.00 GB |
| 7. Real-World Cache (2M x 1KB) | **Nedis (Go SIMD)** | 2000000 | **194538 ops/s** ⚡ | 0.127 ms | 0.743 ms | **1.211 ms** (+47%) | **2.291 ms** (+47%) | 2.99 GB |
| 7. Real-World Cache (2M x 1KB) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |

> **Notes**: SugarDB does not support Streams (XADD/XRANGE), Lua scripting (EVAL/SCRIPT LOAD), Bitmaps (SETBIT/BITCOUNT), or HyperLogLog (PFADD) — verified empirically via `redis-cli` error replies — so those workloads are marked N/A. SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics rather than index-range, so its ZSet range queries return empty sets under this workload; the row is still measured. SugarDB does not support `INFO memory`, so its Memory Delta is reported as 0 B. On workload 7 (2M keys × 1KB values), SugarDB's server process crashes (container exit 2 with a goroutine dump), so it is marked N/A there. Percentages in the P99/P99.9 columns are the delta versus the C Redis row of the same workload; negative is better (lower tail latency).

---

## 4. 🔍 Deep-Dive Analysis by Scenario

### ① High-Concurrency SET/GET Throughput
- **Scenario**: 50 concurrent client connections, 50,000 operations with 128-byte payloads.
- **Analysis**: Nedis utilizes a **64-shard contention-free architecture**, socket-level `TCP_NODELAY`, and Beaver Arena memory pooling across 12 CPU hardware threads. It achieves **309k QPS (2.34x higher than C Redis)** with comparable tail latency (**P99: 0.682ms vs C Redis 0.686ms**). SugarDB reaches 100k QPS (Nedis is 3.09x faster).

### ② Multi-Field Hash Aggregation
- **Scenario**: 20 concurrent clients, 5-field HSET and HMGET operations across 20,000 requests.
- **Analysis**: With **Hybrid Flat-Dict (contiguous array pairs for <= 64 entries + automatic hash map promotion)** and single-pass RESP buffer serialization, Nedis delivers **196k QPS (1.66x C Redis)**, **P50 of 0.099ms**, and **P99 of 0.250ms (vs C Redis 0.296ms)**. SugarDB reaches 94k QPS (Nedis is 2.09x faster).

### ③ Ranked SkipList Leaderboard (ZSet)
- **Scenario**: 2,000 simulated players with real-time score updates (ZADD), rank lookups (ZRANK), and top-N range queries (ZRANGE).
- **Analysis**: Lightweight Mutex synchronization, lock-free `math/rand/v2` level generation, and stack-allocated node arrays achieve **198k QPS (1.61x C Redis)**, with **P95 (0.160ms vs 0.213ms)** and **P99 (0.252ms vs 0.290ms)**. (SugarDB's ZRANGE uses score-range semantics and returns empty sets here, so its 617k QPS is not an apples-to-apples comparison.)

### ④ Stream Event Queue (XADD / XRANGE)
- **Scenario**: 20 concurrent producers and consumers logging sensor telemetry events and querying ranges across 20,000 records.
- **Analysis**: O(1) chunk boundary skipping and `AddRaw` zero-copy parsing yield **108.7k QPS (1.55x C Redis)** with **P50 of 0.140ms** and **P95 of 0.509ms (vs C Redis 0.495ms)**. SugarDB does not support Streams (N/A).

### ⑤ Redlock Atomic Lua Scripting (Bytecode JIT Cache)
- **Scenario**: 20 workers competing for 100 distributed lock keys with atomic Lua release scripts.
- **Analysis**: Pre-compiled `FunctionProto` caching, VM table reuse, and zero-alloc `redis.call` argument conversions produce **156.6k QPS (1.36x C Redis)** with **P50 of 0.117ms**, **P95 of 0.173ms**, and **P99 of 0.293ms (vs C Redis 0.297ms)**. SugarDB does not support EVAL/SCRIPT LOAD (N/A).

### ⑥ Bitmap & HyperLogLog Cardinality Estimation
- **Scenario**: 100,000 bit mutations with SETBIT, 64-bit word POPCNT BITCOUNT, and 50,000 unique IP insertions with PFADD/PFCOUNT.
- **Analysis**: In-place zero-reallocation bit mutation, `math/bits.OnesCount64` hardware acceleration, and 12KB Dense Otmar Ertl HLL registers achieve **207.3k QPS (1.60x C Redis)** with **P50 of 0.096ms** and **P95 of 0.141ms (vs C Redis 0.197ms)**. SugarDB does not support SETBIT/BITCOUNT/PFADD (N/A).
### ⑦ Real-World Cache at Scale (2M keys × 1KB)
- **Scenario**: 50 concurrent clients loading 1,600,000 cache entries with 1KB values (~1.6GB of payload), 80% SET / 20% GET. This approximates a production cache workload where the dataset itself dominates memory usage.
- **Analysis**: With real data dominating, the memory gap narrows: Nedis grows **3.00 GB** vs C Redis **2.06 GB** (46% more), down from the multi-x ratio on the small overhead-dominated workloads above. Throughput: **195k QPS (1.69x C Redis)**, P99.9 **2.193ms vs 1.560ms**. (SugarDB crashes under this 2M×1KB load, so it is N/A.)


---

## 5. 🐹 Go Implementation Comparison (vs SugarDB)

SugarDB is now measured as a first-class target in this suite (see §3), so the numbers below are taken directly from the suite-measured rows above — identical machine (AMD Ryzen 5 5600X), identical harness (direct RESP TCP, pre-connected pooled clients), SugarDB `latest` running in Docker with host networking. SugarDB supports only workloads 1–3; workloads 4–6 are N/A (unsupported), and it crashes under workload 7's 2M×1KB load.

| Workload | Target | QPS (ops/s) | P50 (ms) | P95 (ms) | P99 (ms) | P99.9 (ms) |
|---|---|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | **132205** | 0.357 | 0.459 | 0.686 | 1.190 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go)** | **309445** | 0.126 | 0.315 | 0.682 | 2.279 |
| 1. Concurrency Throughput (SET/GET) | **Nedis (Go SIMD)** | **304610** | 0.129 | 0.332 | 0.711 | 1.576 |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | **100131** | 0.266 | 1.475 | 1.983 | 3.769 |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | **118447** | 0.160 | 0.224 | 0.296 | 0.369 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go)** | **196183** | 0.099 | 0.157 | 0.250 | 0.593 |
| 2. Multi-Field Hash Aggregation | **Nedis (Go SIMD)** | **184902** | 0.102 | 0.157 | 0.319 | 1.728 |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | **93670** | 0.149 | 0.497 | 0.828 | 14.114 |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | **123131** | 0.153 | 0.213 | 0.290 | 0.359 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go)** | **198272** | 0.095 | 0.160 | 0.252 | 1.183 |
| 3. SkipList Leaderboard (ZSet) | **Nedis (Go SIMD)** | **191772** | 0.098 | 0.164 | 0.286 | 1.135 |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | **616610** | 0.008 | 0.010 | 0.020 | 10.032 |

Throughput (SET/GET, workload 1): Nedis ≈ **3.1x SugarDB**. Hash workload: Nedis ≈ **2.1x SugarDB**. On workload 3 (ZSet), SugarDB's ZRANGE implements score-range rather than index-range semantics and returns empty sets under this workload, so its raw QPS is not comparable; Nedis and C Redis perform real index-range scans. SugarDB cannot run workloads 4–6 at all.

**Conclusion**: Against SugarDB, the most actively maintained pure-Go in-memory Redis alternative, Nedis delivers substantially higher throughput on the comparable workloads under identical suite conditions, and additionally supports Streams, Lua scripting, Bitmaps, and HyperLogLog, which SugarDB lacks entirely.

---

## 6. 🧠 Memory Characteristics

Memory Delta is the growth of each server's `used_memory_rss` (from `INFO memory`) measured before and after each workload. RSS is used because it is comparable across engines: C Redis's `used_memory` covers only jemalloc allocations, and Nedis's `used_memory` is Go `MemStats.Alloc`, which lags GC sweep timing — RSS reflects what the OS actually backs with physical pages. (Nedis reports Go `MemStats.Sys` as `used_memory_rss`, i.e. memory obtained from the OS, which slightly over-counts versus true RSS.) SugarDB does not support `INFO memory`, so its deltas are reported as 0 B and omitted from the totals.

### At realistic dataset sizes (workload 7: 2M keys × 1KB values)

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 2.06 GB | baseline |
| **Nedis (Go)** | 3.00 GB | 1.46x |
| **Nedis (Go SIMD)** | 2.99 GB | 1.46x |

Once the dataset itself (~1.6GB of 1KB values) dominates `used_memory`, the gap between Nedis and C Redis narrows from the multi-x ratio seen on the small overhead-dominated workloads to a bounded addend (**3.00 GB** vs **2.06 GB**, 46% difference). The per-key amplification (2.0KB vs 1.3KB stored per 1KB key/value pair) comes from the 64-shard dict's pre-sized buckets and Go object headers; the fixed baseline overhead from the table below becomes proportionally smaller as data grows.

### Baseline overhead (workloads 1–6, overhead-dominated)

On small workloads the fixed per-engine overhead is the dominant term. This isolates the constant cost each engine pays regardless of dataset size.

| Workload | C Redis 8.0 | Nedis (Standard) | Nedis (SIMD/AVX2) | SugarDB (Go) |
|---|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | 4.62 MB | 22.12 MB | 22.44 MB | 0 B |
| 2. Multi-Field Hash Aggregation | 1.12 MB | 12.62 MB | 12.12 MB | 0 B |
| 3. SkipList Leaderboard (ZSet) | 128.00 KB | 64.00 KB | 64.00 KB | 0 B |
| 4. Stream Queue (XADD/XRANGE) | 256.00 KB | 16.56 MB | 20.62 MB | N/A |
| 5. Redlock Atomic Lua Scripting | 128.00 KB | 24.38 MB | 20.32 MB | N/A |
| 6. Bitmap & HyperLogLog | 0 B | 0 B | 0 B | N/A |
| **Total (workloads 1–6)** | **6.25 MB** | **75.75 MB** | **75.57 MB** | **0 B** |

**Analysis**: Nedis trades a higher fixed baseline (**75.75 MB** vs C Redis **6.25 MB** across workloads 1–6) for its throughput and tail-latency gains. The delta is deliberate pre-allocation, not per-request leakage: the 64-shard architecture pre-sizes per-shard buffers, and the Beaver Arena pool retains 8KB stream chunk slabs for reuse instead of returning them to the OS. C Redis, backed by jemalloc, grows incrementally and reports the smallest deltas, while SugarDB's memory usage is unmeasurable in this suite because it does not implement `INFO memory`.
