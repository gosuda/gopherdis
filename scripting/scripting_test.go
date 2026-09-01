package scripting

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestEngine_BasicEval(t *testing.T) {
	eng := NewEngine()

	// Mock in-memory key-value store for redis.call
	kv := make(map[string]string)
	var mu sync.Mutex

	exec := func(argv [][]byte) []byte {
		mu.Lock()
		defer mu.Unlock()

		cmd := strings.ToUpper(string(argv[0]))
		switch cmd {
		case "SET":
			kv[string(argv[1])] = string(argv[2])
			return []byte("+OK\r\n")
		case "GET":
			val, ok := kv[string(argv[1])]
			if !ok {
				return []byte("$-1\r\n")
			}
			return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val))
		case "DEL":
			delete(kv, string(argv[1]))
			return []byte(":1\r\n")
		default:
			return []byte("-ERR unknown command\r\n")
		}
	}

	// 1. Redlock unlock script
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	// Initial key
	kv["lock:order_1"] = "token_abc"

	// Match token -> returns :1
	res, err := eng.Eval(script, []string{"lock:order_1"}, []string{"token_abc"}, exec)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}

	// Mismatch token -> returns :0
	kv["lock:order_1"] = "token_xyz"
	res, err = eng.Eval(script, []string{"lock:order_1"}, []string{"token_abc"}, exec)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}
	if string(res) != ":0\r\n" {
		t.Fatalf("expected :0, got %q", res)
	}
}

func TestEngine_ConcurrentMultiVM(t *testing.T) {
	eng := NewEngine()

	script := `
		local k = KEYS[1]
		local v = ARGV[1]
		redis.call("SET", k, v)
		return redis.call("GET", k)
	`

	var kv sync.Map
	exec := func(argv [][]byte) []byte {
		cmd := strings.ToUpper(string(argv[0]))
		switch cmd {
		case "SET":
			kv.Store(string(argv[1]), string(argv[2]))
			return []byte("+OK\r\n")
		case "GET":
			val, ok := kv.Load(string(argv[1]))
			if !ok {
				return []byte("$-1\r\n")
			}
			s := val.(string)
			return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(s), s))
		default:
			return []byte("-ERR unknown\r\n")
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key_%d", id)
			val := fmt.Sprintf("val_%d", id)
			res, err := eng.Eval(script, []string{key}, []string{val}, exec)
			if err != nil {
				t.Errorf("routine %d failed: %v", id, err)
				return
			}
			expected := fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)
			if string(res) != expected {
				t.Errorf("routine %d mismatch: got %q, expected %q", id, res, expected)
			}
		}(i)
	}
	wg.Wait()
}
