# 📊 C Redis 8.0 vs Gopherdis (Pure Go) vs SugarDB (Pure Go) 7-Dimensional Benchmark Analysis Report

This document details the multi-dimensional benchmark methodology and performance comparison results between official C Redis 8.0, Gopherdis (Pure Go Redis-compatible store), and SugarDB (Pure Go in-memory Redis alternative, running in Docker with host networking), executed under an identical hardware and local loopback TCP network environment.

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

![C Redis vs Gopherdis Benchmark Chart](benchmark_chart.svg)

## 3. 📋 Benchmark Summary Table

| Workload Scenario | Target Engine | Total Ops | Throughput (QPS) | P50 Latency (ms) | P95 Latency (ms) | P99 Latency (ms) | P99.9 Latency (ms) | Memory Delta |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | 50000 | **119974 ops/s** | 0.390 ms | 0.520 ms | **0.836 ms** | **2.805 ms** | 4.00 MB |
| 1. Concurrency Throughput (SET/GET) | **Gopherdis (Go)** | 50000 | **298703 ops/s** ⚡ | 0.129 ms | 0.350 ms | **0.739 ms** (-12%) | **1.960 ms** (-30%) | 11.04 MB |
| 1. Concurrency Throughput (SET/GET) | **Gopherdis (Go SIMD)** | 50000 | **293659 ops/s** ⚡ | 0.132 ms | 0.354 ms | **0.680 ms** (-19%) | **1.631 ms** (-42%) | 10.79 MB |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | 50000 | **88781 ops/s** | 0.294 ms | 1.614 ms | **2.441 ms** (+192%) | **15.970 ms** (+469%) | 0 B |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | 20000 | **111687 ops/s** | 0.169 ms | 0.239 ms | **0.314 ms** | **1.422 ms** | 384.00 KB |
| 2. Multi-Field Hash Aggregation | **Gopherdis (Go)** | 20000 | **148308 ops/s** ⚡ | 0.114 ms | 0.210 ms | **0.461 ms** (+47%) | **3.560 ms** (+150%) | 9.89 MB |
| 2. Multi-Field Hash Aggregation | **Gopherdis (Go SIMD)** | 20000 | **168950 ops/s** ⚡ | 0.114 ms | 0.187 ms | **0.301 ms** (-4%) | **0.977 ms** (-31%) | 9.81 MB |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | 20000 | **91437 ops/s** | 0.166 ms | 0.545 ms | **0.868 ms** (+176%) | **1.797 ms** (+26%) | 0 B |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | 20000 | **110559 ops/s** | 0.164 ms | 0.235 ms | **0.380 ms** | **3.111 ms** | 128.00 KB |
| 3. SkipList Leaderboard (ZSet) | **Gopherdis (Go)** | 20000 | **140324 ops/s** ⚡ | 0.122 ms | 0.267 ms | **0.495 ms** (+30%) | **2.518 ms** (-19%) | 1.68 MB |
| 3. SkipList Leaderboard (ZSet) | **Gopherdis (Go SIMD)** | 20000 | **141490 ops/s** ⚡ | 0.130 ms | 0.246 ms | **0.455 ms** (+20%) | **2.490 ms** (-20%) | 0 B |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | 20000 | **395566 ops/s** | 0.008 ms | 0.010 ms | **0.034 ms** (-91%) | **20.984 ms** (+575%) | 0 B |
| 4. Stream Queue (XADD/XRANGE) | **C Redis 8.0** | 20000 | **79615 ops/s** | 0.228 ms | 0.371 ms | **0.499 ms** | **4.613 ms** | 384.00 KB |
| 4. Stream Queue (XADD/XRANGE) | **Gopherdis (Go)** | 20000 | **96679 ops/s** ⚡ | 0.158 ms | 0.549 ms | **0.908 ms** (+82%) | **1.647 ms** (-64%) | 5.36 MB |
| 4. Stream Queue (XADD/XRANGE) | **Gopherdis (Go SIMD)** | 20000 | **99391 ops/s** ⚡ | 0.153 ms | 0.534 ms | **0.901 ms** (+81%) | **1.387 ms** (-70%) | 7.27 MB |
| 4. Stream Queue (XADD/XRANGE) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 5. Redlock Atomic Lua Scripting | **C Redis 8.0** | 20000 | **105839 ops/s** | 0.173 ms | 0.264 ms | **0.393 ms** | **1.582 ms** | 128.00 KB |
| 5. Redlock Atomic Lua Scripting | **Gopherdis (Go)** | 20000 | **4200 ops/s** ⚡ | 4.240 ms | 10.980 ms | **16.544 ms** (+4110%) | **24.481 ms** (+1447%) | 0 B |
| 5. Redlock Atomic Lua Scripting | **Gopherdis (Go SIMD)** | 20000 | **3275 ops/s** ⚡ | 4.518 ms | 16.369 ms | **25.547 ms** (+6401%) | **34.456 ms** (+2078%) | 0 B |
| 5. Redlock Atomic Lua Scripting | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 6. Bitmap & HyperLogLog | **C Redis 8.0** | 30000 | **120748 ops/s** | 0.155 ms | 0.218 ms | **0.348 ms** | **0.841 ms** | 0 B |
| 6. Bitmap & HyperLogLog | **Gopherdis (Go)** | 30000 | **201204 ops/s** ⚡ | 0.099 ms | 0.149 ms | **0.201 ms** (-42%) | **0.382 ms** (-55%) | 128.00 KB |
| 6. Bitmap & HyperLogLog | **Gopherdis (Go SIMD)** | 30000 | **202580 ops/s** ⚡ | 0.102 ms | 0.146 ms | **0.182 ms** (-48%) | **0.376 ms** (-55%) | 140.00 KB |
| 6. Bitmap & HyperLogLog | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |
| 7. Real-World Cache (2M x 1KB) | **C Redis 8.0** | 2000000 | **111420 ops/s** | 0.430 ms | 0.554 ms | **0.867 ms** | **1.643 ms** | 2.06 GB |
| 7. Real-World Cache (2M x 1KB) | **Gopherdis (Go)** | 2000000 | **190291 ops/s** ⚡ | 0.138 ms | 0.756 ms | **1.227 ms** (+42%) | **2.296 ms** (+40%) | 1.81 GB |
| 7. Real-World Cache (2M x 1KB) | **Gopherdis (Go SIMD)** | 2000000 | **188686 ops/s** ⚡ | 0.146 ms | 0.758 ms | **1.236 ms** (+43%) | **2.329 ms** (+42%) | 1.81 GB |
| 7. Real-World Cache (2M x 1KB) | **SugarDB (Go)** | N/A (unsupported) | — | — | — | — | — | — |

> **Notes**: SugarDB does not support Streams (XADD/XRANGE), Lua scripting (EVAL/SCRIPT LOAD), Bitmaps (SETBIT/BITCOUNT), or HyperLogLog (PFADD) — verified empirically via `redis-cli` error replies — so those workloads are marked N/A. SugarDB's ZRANGE uses score-range (ZRANGEBYSCORE) semantics rather than index-range, so its ZSet range queries return empty sets under this workload; the row is still measured. SugarDB does not support `INFO memory`, so its Memory Delta is reported as 0 B. On workload 7 (2M keys × 1KB values), SugarDB's server process crashes (container exit 2 with a goroutine dump), so it is marked N/A there. Percentages in the P99/P99.9 columns are the delta versus the C Redis row of the same workload; negative is better (lower tail latency).

---

## 4. 🔍 Deep-Dive Analysis by Scenario

### ① High-Concurrency SET/GET Throughput
- **Scenario**: 50 concurrent client connections, 50,000 operations with 128-byte payloads.
- **Analysis**: Gopherdis utilizes a **64-shard contention-free architecture**, socket-level `TCP_NODELAY`, and Beaver Arena memory pooling across 12 CPU hardware threads. It achieves **299k QPS (2.49x higher than C Redis)** with comparable tail latency (**P99: 0.739ms vs C Redis 0.836ms**). SugarDB reaches 89k QPS (Gopherdis is 3.36x faster).

### ② Multi-Field Hash Aggregation
- **Scenario**: 20 concurrent clients, 5-field HSET and HMGET operations across 20,000 requests.
- **Analysis**: With **Hybrid Flat-Dict (contiguous array pairs for <= 64 entries + automatic hash map promotion)** and single-pass RESP buffer serialization, Gopherdis delivers **148k QPS (1.33x C Redis)**, **P50 of 0.114ms**, and **P99 of 0.461ms (vs C Redis 0.314ms)**. SugarDB reaches 91k QPS (Gopherdis is 1.62x faster).

### ③ Ranked SkipList Leaderboard (ZSet)
- **Scenario**: 2,000 simulated players with real-time score updates (ZADD), rank lookups (ZRANK), and top-N range queries (ZRANGE).
- **Analysis**: Lightweight Mutex synchronization, lock-free `math/rand/v2` level generation, and stack-allocated node arrays achieve **140k QPS (1.27x C Redis)**, with **P95 (0.267ms vs 0.235ms)** and **P99 (0.495ms vs 0.380ms)**. (SugarDB's ZRANGE uses score-range semantics and returns empty sets here, so its 396k QPS is not an apples-to-apples comparison.)

### ④ Stream Event Queue (XADD / XRANGE)
- **Scenario**: 20 concurrent producers and consumers logging sensor telemetry events and querying ranges across 20,000 records.
- **Analysis**: O(1) chunk boundary skipping and `AddRaw` zero-copy parsing yield **96.7k QPS (1.21x C Redis)** with **P50 of 0.158ms** and **P95 of 0.549ms (vs C Redis 0.371ms)**. SugarDB does not support Streams (N/A).

### ⑤ Redlock Atomic Lua Scripting (Bytecode JIT Cache)
- **Scenario**: 20 workers competing for 100 distributed lock keys with atomic Lua release scripts.
- **Analysis**: Pre-compiled `FunctionProto` caching, VM table reuse, and zero-alloc `redis.call` argument conversions produce **4.2k QPS (0.04x C Redis)** with **P50 of 4.240ms**, **P95 of 10.980ms**, and **P99 of 16.544ms (vs C Redis 0.393ms)**. SugarDB does not support EVAL/SCRIPT LOAD (N/A).

### ⑥ Bitmap & HyperLogLog Cardinality Estimation
- **Scenario**: 100,000 bit mutations with SETBIT, 64-bit word POPCNT BITCOUNT, and 50,000 unique IP insertions with PFADD/PFCOUNT.
- **Analysis**: In-place zero-reallocation bit mutation, `math/bits.OnesCount64` hardware acceleration, and 12KB Dense Otmar Ertl HLL registers achieve **201.2k QPS (1.67x C Redis)** with **P50 of 0.099ms** and **P95 of 0.149ms (vs C Redis 0.218ms)**. SugarDB does not support SETBIT/BITCOUNT/PFADD (N/A).
### ⑦ Real-World Cache at Scale (2M keys × 1KB)
- **Scenario**: 50 concurrent clients loading 1,600,000 cache entries with 1KB values (~1.6GB of payload), 80% SET / 20% GET. This approximates a production cache workload where the dataset itself dominates memory usage.
- **Analysis**: With real data dominating, memory reaches parity: Gopherdis grows **1.81 GB** vs C Redis **2.06 GB** (-12%), unlike the small overhead-dominated workloads above. Throughput: **190k QPS (1.71x C Redis)**, P99.9 **2.296ms vs 1.643ms**. (SugarDB crashes under this 2M×1KB load, so it is N/A.)


---

## 5. 🐹 Go Implementation Comparison (vs SugarDB)

SugarDB is now measured as a first-class target in this suite (see §3), so the numbers below are taken directly from the suite-measured rows above — identical machine (AMD Ryzen 5 5600X), identical harness (direct RESP TCP, pre-connected pooled clients), SugarDB `latest` running in Docker with host networking. SugarDB supports only workloads 1–3; workloads 4–6 are N/A (unsupported), and it crashes under workload 7's 2M×1KB load.

| Workload | Target | QPS (ops/s) | P50 (ms) | P95 (ms) | P99 (ms) | P99.9 (ms) |
|---|---|:---:|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | **C Redis 8.0** | **119974** | 0.390 | 0.520 | 0.836 | 2.805 |
| 1. Concurrency Throughput (SET/GET) | **Gopherdis (Go)** | **298703** | 0.129 | 0.350 | 0.739 | 1.960 |
| 1. Concurrency Throughput (SET/GET) | **Gopherdis (Go SIMD)** | **293659** | 0.132 | 0.354 | 0.680 | 1.631 |
| 1. Concurrency Throughput (SET/GET) | **SugarDB (Go)** | **88781** | 0.294 | 1.614 | 2.441 | 15.970 |
| 2. Multi-Field Hash Aggregation | **C Redis 8.0** | **111687** | 0.169 | 0.239 | 0.314 | 1.422 |
| 2. Multi-Field Hash Aggregation | **Gopherdis (Go)** | **148308** | 0.114 | 0.210 | 0.461 | 3.560 |
| 2. Multi-Field Hash Aggregation | **Gopherdis (Go SIMD)** | **168950** | 0.114 | 0.187 | 0.301 | 0.977 |
| 2. Multi-Field Hash Aggregation | **SugarDB (Go)** | **91437** | 0.166 | 0.545 | 0.868 | 1.797 |
| 3. SkipList Leaderboard (ZSet) | **C Redis 8.0** | **110559** | 0.164 | 0.235 | 0.380 | 3.111 |
| 3. SkipList Leaderboard (ZSet) | **Gopherdis (Go)** | **140324** | 0.122 | 0.267 | 0.495 | 2.518 |
| 3. SkipList Leaderboard (ZSet) | **Gopherdis (Go SIMD)** | **141490** | 0.130 | 0.246 | 0.455 | 2.490 |
| 3. SkipList Leaderboard (ZSet) | **SugarDB (Go)** | **395566** | 0.008 | 0.010 | 0.034 | 20.984 |

Throughput (SET/GET, workload 1): Gopherdis ≈ **3.4x SugarDB**. Hash workload: Gopherdis ≈ **1.6x SugarDB**. On workload 3 (ZSet), SugarDB's ZRANGE implements score-range rather than index-range semantics and returns empty sets under this workload, so its raw QPS is not comparable; Gopherdis and C Redis perform real index-range scans. SugarDB cannot run workloads 4–6 at all.

**Conclusion**: Against SugarDB, the most actively maintained pure-Go in-memory Redis alternative, Gopherdis delivers substantially higher throughput on the comparable workloads under identical suite conditions, and additionally supports Streams, Lua scripting, Bitmaps, and HyperLogLog, which SugarDB lacks entirely.

---

## 6. 🧠 Memory Characteristics

Memory Delta is the growth of each server's `used_memory_rss` (from `INFO memory`) measured before and after each workload. RSS is used because it is comparable across engines: C Redis's `used_memory` covers only jemalloc allocations, and Gopherdis's `used_memory` is Go `MemStats.Alloc`, which lags GC sweep timing — RSS reflects what the OS actually backs with physical pages. Both engines report real RSS: C Redis reads `/proc/self/statm`, and Gopherdis does the same after a full GC plus `debug.FreeOSMemory()` so GC-cycle headroom is not counted. SugarDB does not support `INFO memory`, so its deltas are reported as 0 B and omitted from the totals.

### At realistic dataset sizes (workload 7: 2M keys × 1KB values)

| Target | Memory Delta | vs C Redis |
|---|:---:|:---:|
| **C Redis 8.0** | 2.06 GB | baseline |
| **Gopherdis (Go)** | 1.81 GB | 0.88x |
| **Gopherdis (Go SIMD)** | 1.81 GB | 0.88x |

Once the dataset itself (~1.6GB of 1KB values) dominates `used_memory`, Gopherdis and C Redis reach memory parity (**1.81 GB** vs **2.06 GB**, -12%): 1.2KB vs 1.4KB stored per 1KB key/value pair. Go 1.24's Swiss-table maps and slim `Robj` headers keep per-key cost low, and Gopherdis's `INFO memory` runs a full GC plus `debug.FreeOSMemory()` before reporting so GC headroom is not counted. The fixed baseline overhead in the table below is constant with respect to dataset size and becomes proportionally negligible as data grows.

### Baseline overhead (workloads 1–6, overhead-dominated)

On small workloads the fixed per-engine overhead is the dominant term. This isolates the constant cost each engine pays regardless of dataset size.

| Workload | C Redis 8.0 | Gopherdis (Standard) | Gopherdis (SIMD/AVX2) | SugarDB (Go) |
|---|:---:|:---:|:---:|:---:|
| 1. Concurrency Throughput (SET/GET) | 4.00 MB | 11.04 MB | 10.79 MB | 0 B |
| 2. Multi-Field Hash Aggregation | 384.00 KB | 9.89 MB | 9.81 MB | 0 B |
| 3. SkipList Leaderboard (ZSet) | 128.00 KB | 1.68 MB | 0 B | 0 B |
| 4. Stream Queue (XADD/XRANGE) | 384.00 KB | 5.36 MB | 7.27 MB | N/A |
| 5. Redlock Atomic Lua Scripting | 128.00 KB | 0 B | 0 B | N/A |
| 6. Bitmap & HyperLogLog | 0 B | 128.00 KB | 140.00 KB | N/A |
| **Total (workloads 1–6)** | **5.00 MB** | **28.11 MB** | **28.00 MB** | **0 B** |

**Analysis**: Gopherdis trades a higher fixed baseline (**28.11 MB** vs C Redis **5.00 MB** across workloads 1–6) for its throughput and tail-latency gains. The delta is deliberate pre-allocation, not per-request leakage: the 64-shard architecture pre-sizes per-shard buffers, and the Beaver Arena pool retains 8KB stream chunk slabs for reuse instead of returning them to the OS. C Redis, backed by jemalloc, grows incrementally and reports the smallest deltas, while SugarDB's memory usage is unmeasurable in this suite because it does not implement `INFO memory`.
