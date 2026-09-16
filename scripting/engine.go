package scripting

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

var (
	ErrNoSuchScript  = errors.New("NOSCRIPT No matching script. Please use EVAL.")
	ErrScriptTimeout = errors.New("BUSY Script exceeded the maximum execution time and was terminated")
)

// ScriptTimeout bounds how long one EVAL/EVALSHA may run. Scripts execute while
// holding the database's exclusive transaction lock, so an unbounded script
// ("while true do end") freezes every other client permanently.
var ScriptTimeout = 5 * time.Second

// CommandExecutor is a callback function allowing Lua to invoke server commands.
type CommandExecutor func(argv [][]byte) []byte

const (
	// maxPooledVMs bounds how many idle interpreters are kept alive.
	maxPooledVMs = 16
	// maxVMFunctions bounds the per-VM compiled-function cache before it is
	// dropped wholesale, so a client loading endless scripts cannot grow it
	// without limit.
	maxVMFunctions = 512
	// watchdogInterval is how often running scripts are checked against their
	// deadline.
	watchdogInterval = 100 * time.Millisecond
)

// vm is one Lua interpreter plus the per-interpreter state that makes repeated
// EVALs cheap: the compiled function for each script, and a context installed
// once rather than on every call.
type vm struct {
	L      *lua.LState
	ctx    context.Context
	cancel context.CancelFunc
	ud     *lua.LUserData
	fns    map[string]*lua.LFunction

	// deadline holds the unix-nano deadline of the script currently running on
	// this interpreter, or 0 when it is idle. The watchdog reads it.
	deadline atomic.Int64
}

// Engine manages a pool of isolated Lua VMs for script execution and SHA1 bytecode caching.
type Engine struct {
	mu         sync.RWMutex
	scripts    map[string]string             // SHA1 hex -> script text
	protoCache map[string]*lua.FunctionProto // SHA1 hex -> compiled bytecode proto
	textHashes map[string]string             // script text -> SHA1 hex

	pool chan *vm

	liveMu sync.Mutex
	live   map[*vm]struct{}

	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewEngine initializes the Lua scripting engine with a multi-VM pool and bytecode caching.
func NewEngine() *Engine {
	e := &Engine{
		scripts:    make(map[string]string),
		protoCache: make(map[string]*lua.FunctionProto),
		textHashes: make(map[string]string),
		pool:       make(chan *vm, maxPooledVMs),
		live:       make(map[*vm]struct{}),
		stopCh:     make(chan struct{}),
	}
	go e.watchdog()
	return e
}

// Close stops the engine's watchdog goroutine.
func (e *Engine) Close() {
	e.stopOnce.Do(func() { close(e.stopCh) })
}

// watchdog cancels scripts that have run past ScriptTimeout.
//
// The alternative, arming a context.WithTimeout per EVAL, costs an extra timer,
// cancelCtx and Done channel on every call. Installing one cancellable context
// per interpreter and letting a single ticker enforce the deadline keeps the
// per-call cost at zero allocations.
func (e *Engine) watchdog() {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case now := <-ticker.C:
			nowNano := now.UnixNano()
			e.liveMu.Lock()
			for v := range e.live {
				if d := v.deadline.Load(); d != 0 && nowNano > d {
					v.cancel()
				}
			}
			e.liveMu.Unlock()
		}
	}
}

// getVM takes an interpreter from the pool, creating one if the pool is empty.
func (e *Engine) getVM() *vm {
	select {
	case v := <-e.pool:
		return v
	default:
		return e.createVM()
	}
}

// putVM returns an interpreter to the pool. An interpreter whose context was
// cancelled by the watchdog is destroyed: its stack is in an unknown state and
// its context can never be un-cancelled.
func (e *Engine) putVM(v *vm) {
	if v.ctx.Err() != nil {
		e.destroyVM(v)
		return
	}
	v.L.SetTop(0)
	select {
	case e.pool <- v:
	default:
		e.destroyVM(v)
	}
}

func (e *Engine) destroyVM(v *vm) {
	e.liveMu.Lock()
	delete(e.live, v)
	e.liveMu.Unlock()
	v.cancel()
	v.L.Close()
}

// function returns the compiled LFunction for a script on this interpreter,
// compiling it on first use. An LFunction is bound to the LState that produced
// it, so the cache has to live on the interpreter, not on the Engine.
func (v *vm) function(hash string, proto *lua.FunctionProto) (*lua.LFunction, error) {
	if fn, ok := v.fns[hash]; ok {
		return fn, nil
	}
	if len(v.fns) >= maxVMFunctions {
		v.fns = make(map[string]*lua.LFunction, 16)
	}

	// proto is the wrapper chunk "return function() <script> end"; running it
	// once yields the inner, non-vararg function that each EVAL then calls.
	v.L.Push(v.L.NewFunctionFromProto(proto))
	if err := v.L.PCall(0, 1, nil); err != nil {
		v.L.SetTop(0)
		return nil, err
	}
	fn, ok := v.L.Get(-1).(*lua.LFunction)
	v.L.SetTop(0)
	if !ok {
		return nil, fmt.Errorf("ERR Error compiling script: unexpected chunk result")
	}
	v.fns[hash] = fn
	return fn, nil
}

func (e *Engine) createVM() *vm {
	L := lua.NewState(lua.Options{
		SkipOpenLibs: false,
	})
	// NOTE: the GC is deliberately left running. Disabling it here made every
	// pooled VM accumulate garbage for the lifetime of the process.
	// NOTE: the GC is deliberately left running. Disabling it here made every
	// pooled VM accumulate garbage for the lifetime of the process.

	redisTbl := L.NewTable()

	// redis.call
	L.SetField(redisTbl, "call", L.NewFunction(func(l *lua.LState) int {
		execVal := l.GetGlobal("__exec__")
		ud, ok := execVal.(*lua.LUserData)
		if !ok || ud.Value == nil {
			l.RaiseError("no executor context")
			return 0
		}
		exec := ud.Value.(CommandExecutor)

		top := l.GetTop()
		var argvBuf [8][]byte
		var cmdArgv [][]byte
		if top <= 8 {
			cmdArgv = argvBuf[:0]
		} else {
			cmdArgv = make([][]byte, 0, top)
		}
		for i := 1; i <= top; i++ {
			val := l.Get(i)
			if ls, ok := val.(lua.LString); ok {
				cmdArgv = append(cmdArgv, []byte(string(ls)))
			} else {
				cmdArgv = append(cmdArgv, []byte(val.String()))
			}
		}
		replyBytes := exec(cmdArgv)
		luaVal := RESPToLua(l, bytesTrimCRLF(replyBytes))
		l.Push(luaVal)
		return 1
	}))

	// redis.pcall
	L.SetField(redisTbl, "pcall", L.NewFunction(func(l *lua.LState) int {
		execVal := l.GetGlobal("__exec__")
		ud, ok := execVal.(*lua.LUserData)
		if !ok || ud.Value == nil {
			l.RaiseError("no executor context")
			return 0
		}
		exec := ud.Value.(CommandExecutor)

		top := l.GetTop()
		var argvBuf [8][]byte
		var cmdArgv [][]byte
		if top <= 8 {
			cmdArgv = argvBuf[:0]
		} else {
			cmdArgv = make([][]byte, 0, top)
		}
		for i := 1; i <= top; i++ {
			val := l.Get(i)
			if ls, ok := val.(lua.LString); ok {
				cmdArgv = append(cmdArgv, []byte(string(ls)))
			} else {
				cmdArgv = append(cmdArgv, []byte(val.String()))
			}
		}
		replyBytes := exec(cmdArgv)
		if len(replyBytes) > 0 && replyBytes[0] == '-' {
			errTbl := l.NewTable()
			errTbl.RawSetString("err", lua.LString(string(bytesTrimCRLF(replyBytes[1:]))))
			l.Push(errTbl)
			return 1
		}
		luaVal := RESPToLua(l, bytesTrimCRLF(replyBytes))
		l.Push(luaVal)
		return 1
	}))

	// redis.error_reply
	L.SetField(redisTbl, "error_reply", L.NewFunction(func(l *lua.LState) int {
		msg := l.OptString(1, "ERR")
		tbl := l.NewTable()
		tbl.RawSetString("err", lua.LString(msg))
		l.Push(tbl)
		return 1
	}))

	// redis.status_reply
	L.SetField(redisTbl, "status_reply", L.NewFunction(func(l *lua.LState) int {
		msg := l.OptString(1, "OK")
		tbl := l.NewTable()
		tbl.RawSetString("ok", lua.LString(msg))
		l.Push(tbl)
		return 1
	}))

	// redis.sha1hex
	L.SetField(redisTbl, "sha1hex", L.NewFunction(func(l *lua.LState) int {
		s := l.OptString(1, "")
		l.Push(lua.LString(SHA1(s)))
		return 1
	}))

	L.SetGlobal("redis", redisTbl)

	// One cancellable context per interpreter, installed once. gopher-lua checks
	// it between instructions, which is what makes a runaway script killable.
	ctx, cancel := context.WithCancel(context.Background())
	L.SetContext(ctx)

	ud := L.NewUserData()
	L.SetGlobal("__exec__", ud)

	v := &vm{L: L, ctx: ctx, cancel: cancel, ud: ud, fns: make(map[string]*lua.LFunction, 16)}

	e.liveMu.Lock()
	e.live[v] = struct{}{}
	e.liveMu.Unlock()

	return v
}

// wrapScript turns a script body into a chunk that evaluates to a non-vararg
// function.
//
// A chunk compiled straight from the script text is a vararg main chunk, and
// gopher-lua builds a throwaway `arg` table (two maps and a slice, about 2.8KB)
// on every single call to one. Calling a plain function instead removes that
// entirely. The opening wrapper stays on the first line so reported line numbers
// still match the user's script.
func wrapScript(script string) string {
	return "return function() " + script + "\nend"
}

// SHA1 computes the 40-character SHA1 hex digest of a Lua script.
func SHA1(script string) string {
	sum := sha1.Sum([]byte(script))
	return hex.EncodeToString(sum[:])
}

// CompileOrGet compiles a Lua script into bytecode proto once and caches it.
func (e *Engine) CompileOrGet(script string) (*lua.FunctionProto, string, error) {
	// Look the text up directly first: SHA1 plus hex encoding on every EVAL is
	// pure overhead once a script is known.
	e.mu.RLock()
	if hash, ok := e.textHashes[script]; ok {
		if proto, ok := e.protoCache[hash]; ok {
			e.mu.RUnlock()
			return proto, hash, nil
		}
	}
	e.mu.RUnlock()

	hash := SHA1(script)
	e.mu.RLock()
	proto, ok := e.protoCache[hash]
	e.mu.RUnlock()
	if ok {
		return proto, hash, nil
	}

	chunk, err := parse.Parse(strings.NewReader(wrapScript(script)), "<eval>")
	if err != nil {
		return nil, hash, fmt.Errorf("ERR Error compiling script: %v", err)
	}
	compiled, err := lua.Compile(chunk, "<eval>")
	if err != nil {
		return nil, hash, fmt.Errorf("ERR Error compiling script: %v", err)
	}

	e.mu.Lock()
	e.scripts[hash] = script
	e.protoCache[hash] = compiled
	e.textHashes[script] = hash
	e.mu.Unlock()

	return compiled, hash, nil
}

// LoadScript stores a script in the SHA1 cache and returns its hash.
func (e *Engine) LoadScript(script string) string {
	_, hash, _ := e.CompileOrGet(script)
	return hash
}

// GetScript retrieves a cached script by its SHA1 hash.
func (e *Engine) GetScript(hash string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	script, ok := e.scripts[hash]
	return script, ok
}

// GetProto retrieves a pre-compiled bytecode proto by its SHA1 hash.
func (e *Engine) GetProto(hash string) (*lua.FunctionProto, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	proto, ok := e.protoCache[hash]
	return proto, ok
}

// FlushScripts clears the script and bytecode cache.
func (e *Engine) FlushScripts() {
	e.mu.Lock()
	e.scripts = make(map[string]string)
	e.protoCache = make(map[string]*lua.FunctionProto)
	e.textHashes = make(map[string]string)
	e.mu.Unlock()

	// Drain the pool so no interpreter keeps a compiled function for a script
	// that no longer exists.
	for {
		select {
		case v := <-e.pool:
			e.destroyVM(v)
		default:
			return
		}
	}
}

// ExistsScripts checks existence for a list of SHA1 hashes.
func (e *Engine) ExistsScripts(hashes []string) []bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make([]bool, len(hashes))
	for i, h := range hashes {
		_, res[i] = e.scripts[h]
	}
	return res
}

// Eval executes a Lua script string directly with bytecode caching.
func (e *Engine) Eval(script string, keys []string, args []string, exec CommandExecutor) ([]byte, error) {
	proto, hash, err := e.CompileOrGet(script)
	if err != nil {
		return nil, err
	}
	return e.evalProto(hash, proto, keys, args, exec)
}

// EvalSHA executes a pre-cached Lua script by its SHA1 hash.
func (e *Engine) EvalSHA(hash string, keys []string, args []string, exec CommandExecutor) ([]byte, error) {
	proto, ok := e.GetProto(hash)
	if !ok {
		script, ok := e.GetScript(hash)
		if !ok {
			return nil, ErrNoSuchScript
		}
		var err error
		proto, _, err = e.CompileOrGet(script)
		if err != nil {
			return nil, err
		}
	}
	return e.evalProto(hash, proto, keys, args, exec)
}

func (e *Engine) evalProto(hash string, proto *lua.FunctionProto, keys []string, args []string, exec CommandExecutor) ([]byte, error) {
	v := e.getVM()
	L := v.L

	fn, err := v.function(hash, proto)
	if err != nil {
		e.putVM(v)
		return nil, fmt.Errorf("ERR Error compiling script: %v", err)
	}

	v.ud.Value = exec

	defer func() {
		v.deadline.Store(0)
		v.ud.Value = nil
		e.putVM(v)
	}()

	// 1. Setup KEYS and ARGV global tables (reusing table objects)
	keysTbl := setupTable(L, "KEYS", len(keys))
	for i, k := range keys {
		keysTbl.RawSetInt(i+1, lua.LString(k))
	}
	argvTbl := setupTable(L, "ARGV", len(args))
	for i, a := range args {
		argvTbl.RawSetInt(i+1, lua.LString(a))
	}

	// 2. Arm the deadline the watchdog enforces, then run.
	v.deadline.Store(time.Now().Add(ScriptTimeout).UnixNano())

	L.Push(fn)
	if err := L.PCall(0, 1, nil); err != nil {
		if v.ctx.Err() != nil {
			return nil, ErrScriptTimeout
		}
		return nil, fmt.Errorf("ERR Error running script: %v", err)
	}

	retVal := L.Get(-1)
	return LuaToRESP(retVal), nil
}

// setupTable fetches a global table by name, clearing it for reuse, or creates
// it if the interpreter does not have one yet.
func setupTable(L *lua.LState, name string, n int) *lua.LTable {
	if tb, ok := L.GetGlobal(name).(*lua.LTable); ok {
		for i := tb.Len(); i > 0; i-- {
			tb.RawSetInt(i, lua.LNil)
		}
		return tb
	}
	tb := L.NewTable()
	L.SetGlobal(name, tb)
	return tb
}

func bytesTrimCRLF(b []byte) []byte {
	n := len(b)
	if n >= 2 && b[n-2] == '\r' && b[n-1] == '\n' {
		return b[:n-2]
	}
	return b
}
