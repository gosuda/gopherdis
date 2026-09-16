package commands

import (
	"testing"

	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/scripting"
)

// The two scripts every Redlock client runs: the release (compare-and-delete)
// and the extend (compare-and-pexpire).
const redlockRelease = `if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end`

const redlockExtend = `if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("pexpire", KEYS[1], ARGV[2])
else
	return 0
end`

func benchScript(b *testing.B, script string, args ...string) {
	database := db.NewShardedDB()
	eng := scripting.NewEngine()
	ctx := &Context{DB: database, Scripting: eng}

	DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("lock:res"), []byte("token-abc")})

	argv := [][]byte{[]byte("EVAL"), []byte(script), []byte("1"), []byte("lock:res")}
	for _, a := range args {
		argv = append(argv, []byte(a))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DefaultTable.Execute(ctx, argv)
	}
}

func BenchmarkRedlockRelease(b *testing.B) { benchScript(b, redlockRelease, "wrong-token") }
func BenchmarkRedlockExtend(b *testing.B)  { benchScript(b, redlockExtend, "wrong-token", "30000") }

// A loop-heavy script, where the per-instruction overhead dominates.
func BenchmarkScriptLoop(b *testing.B) {
	benchScript(b, `local n = 0
for i = 1, 200 do n = n + i end
return n`)
}

// Parallel variants: Redlock clients hold locks from many connections at once,
// which is the shape that matters.
func benchScriptParallel(b *testing.B, script string, args ...string) {
	database := db.NewShardedDB()
	eng := scripting.NewEngine()

	DefaultTable.Execute(&Context{DB: database, Scripting: eng},
		[][]byte{[]byte("SET"), []byte("lock:res"), []byte("token-abc")})

	argv := [][]byte{[]byte("EVAL"), []byte(script), []byte("1"), []byte("lock:res")}
	for _, a := range args {
		argv = append(argv, []byte(a))
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := &Context{DB: database, Scripting: eng}
		for pb.Next() {
			DefaultTable.Execute(ctx, argv)
		}
	})
}

func BenchmarkRedlockReleaseParallel(b *testing.B) {
	benchScriptParallel(b, redlockRelease, "wrong-token")
}

// Baseline: the same compare-and-delete expressed as plain commands.
func BenchmarkPlainGetParallel(b *testing.B) {
	database := db.NewShardedDB()
	DefaultTable.Execute(&Context{DB: database}, [][]byte{[]byte("SET"), []byte("lock:res"), []byte("token-abc")})
	argv := [][]byte{[]byte("GET"), []byte("lock:res")}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := &Context{DB: database}
		for pb.Next() {
			DefaultTable.Execute(ctx, argv)
		}
	})
}
