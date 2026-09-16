package scripting

import (
	"testing"
	"time"
)

// The release and extend scripts every Redlock client runs.
const (
	redlockRelease = `if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end`
	redlockExtend = `if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("pexpire", KEYS[1], ARGV[2])
else
	return 0
end`
)

// TestEvalAllocationBudget pins the per-EVAL allocation count.
//
// A chunk compiled straight from script text is a vararg main chunk, and
// gopher-lua builds a throwaway `arg` table (two maps and a slice, roughly
// 2.8KB) every time one is called. Wrapping the body in a plain function
// removes that. This guards against the wrapper being dropped: without it the
// budget below is exceeded by a wide margin.
func TestEvalAllocationBudget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		args   []string
		budget float64
	}{
		{"release", redlockRelease, []string{"tok"}, 18},
		{"extend", redlockExtend, []string{"tok", "30000"}, 20},
		{"trivial", `return 1`, nil, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEngine()
			defer e.Close()
			exec := func(argv [][]byte) []byte { return []byte("$3\r\ntok\r\n") }

			// Warm the caches: first call compiles and binds the function.
			if _, err := e.Eval(tc.script, []string{"k"}, tc.args, exec); err != nil {
				t.Fatal(err)
			}

			got := testing.AllocsPerRun(200, func() {
				if _, err := e.Eval(tc.script, []string{"k"}, tc.args, exec); err != nil {
					t.Fatal(err)
				}
			})
			if got > tc.budget {
				t.Fatalf("EVAL allocated %.0f objects per run, budget is %.0f", got, tc.budget)
			}
			t.Logf("%s: %.0f allocs/eval (budget %.0f)", tc.name, got, tc.budget)
		})
	}
}

// TestVMReuse checks that repeated EVALs share interpreters instead of building
// a new one each time.
func TestVMReuse(t *testing.T) {
	e := NewEngine()
	defer e.Close()
	exec := func(argv [][]byte) []byte { return []byte("$3\r\ntok\r\n") }

	for i := 0; i < 500; i++ {
		if _, err := e.Eval(redlockRelease, []string{"k"}, []string{"tok"}, exec); err != nil {
			t.Fatal(err)
		}
	}
	e.liveMu.Lock()
	n := len(e.live)
	e.liveMu.Unlock()
	if n > 2 {
		t.Fatalf("500 sequential evals built %d interpreters, want at most 2", n)
	}
}

// TestRunawayScriptIsKilled covers the watchdog replacing the per-call context.
func TestRunawayScriptIsKilled(t *testing.T) {
	old := ScriptTimeout
	ScriptTimeout = 150 * time.Millisecond
	defer func() { ScriptTimeout = old }()

	e := NewEngine()
	defer e.Close()
	exec := func(argv [][]byte) []byte { return []byte("+OK\r\n") }

	done := make(chan error, 1)
	go func() {
		_, err := e.Eval(`while true do end`, nil, nil, exec)
		done <- err
	}()

	select {
	case err := <-done:
		if err != ErrScriptTimeout {
			t.Fatalf("want ErrScriptTimeout, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runaway script was never killed")
	}

	// The engine must still work afterwards, on a fresh interpreter.
	if _, err := e.Eval(`return 1`, nil, nil, exec); err != nil {
		t.Fatalf("engine unusable after a killed script: %v", err)
	}
}
