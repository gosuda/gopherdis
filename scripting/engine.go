package scripting

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

var (
	ErrNoSuchScript = errors.New("NOSCRIPT No matching script. Please use EVAL.")
)

// CommandExecutor is a callback function allowing Lua to invoke server commands.
type CommandExecutor func(argv [][]byte) []byte

// Engine manages an elastic pool of isolated Lua VMs for concurrent script execution and SHA1 bytecode caching.
type Engine struct {
	vmPool     sync.Pool
	mu         sync.RWMutex
	scripts    map[string]string             // SHA1 hex -> script text
	protoCache map[string]*lua.FunctionProto // SHA1 hex -> compiled bytecode proto
}

// NewEngine initializes the Lua scripting engine with a multi-VM pool and bytecode caching.
func NewEngine() *Engine {
	e := &Engine{
		scripts:    make(map[string]string),
		protoCache: make(map[string]*lua.FunctionProto),
	}
	e.vmPool = sync.Pool{
		New: func() any {
			return e.createVM()
		},
	}
	return e
}

func (e *Engine) createVM() *lua.LState {
	L := lua.NewState(lua.Options{
		SkipOpenLibs: false,
	})
	_ = L.DoString("collectgarbage('stop')")

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
	return L
}

// SHA1 computes the 40-character SHA1 hex digest of a Lua script.
func SHA1(script string) string {
	sum := sha1.Sum([]byte(script))
	return hex.EncodeToString(sum[:])
}

// CompileOrGet compiles a Lua script into bytecode proto once and caches it.
func (e *Engine) CompileOrGet(script string) (*lua.FunctionProto, string, error) {
	hash := SHA1(script)
	e.mu.RLock()
	proto, ok := e.protoCache[hash]
	e.mu.RUnlock()
	if ok {
		return proto, hash, nil
	}

	chunk, err := parse.Parse(strings.NewReader(script), "<eval>")
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
	e.mu.Unlock()
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
	proto, _, err := e.CompileOrGet(script)
	if err != nil {
		return nil, err
	}
	return e.evalProto(proto, keys, args, exec)
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
	return e.evalProto(proto, keys, args, exec)
}

func (e *Engine) evalProto(proto *lua.FunctionProto, keys []string, args []string, exec CommandExecutor) ([]byte, error) {
	L := e.vmPool.Get().(*lua.LState)

	udVal := L.GetGlobal("__exec__")
	var ud *lua.LUserData
	if u, ok := udVal.(*lua.LUserData); ok {
		ud = u
	} else {
		ud = L.NewUserData()
		L.SetGlobal("__exec__", ud)
	}
	ud.Value = exec

	defer func() {
		L.SetTop(0)
		ud.Value = nil
		e.vmPool.Put(L)
	}()

	// 1. Setup KEYS and ARGV global tables (reusing table objects)
	keysVal := L.GetGlobal("KEYS")
	var keysTbl *lua.LTable
	if kt, ok := keysVal.(*lua.LTable); ok {
		keysTbl = kt
		oldLen := keysTbl.Len()
		for i := 1; i <= oldLen; i++ {
			keysTbl.RawSetInt(i, lua.LNil)
		}
	} else {
		keysTbl = L.NewTable()
		L.SetGlobal("KEYS", keysTbl)
	}
	for i, k := range keys {
		keysTbl.RawSetInt(i+1, lua.LString(k))
	}

	argvVal := L.GetGlobal("ARGV")
	var argvTbl *lua.LTable
	if at, ok := argvVal.(*lua.LTable); ok {
		argvTbl = at
		oldLen := argvTbl.Len()
		for i := 1; i <= oldLen; i++ {
			argvTbl.RawSetInt(i, lua.LNil)
		}
	} else {
		argvTbl = L.NewTable()
		L.SetGlobal("ARGV", argvTbl)
	}
	for i, a := range args {
		argvTbl.RawSetInt(i+1, lua.LString(a))
	}

	// 3. Execute pre-compiled FunctionProto
	fn := L.NewFunctionFromProto(proto)
	L.Push(fn)

	err := L.PCall(0, 1, nil)
	if err != nil {
		return nil, fmt.Errorf("ERR Error running script: %v", err)
	}

	retVal := L.Get(-1)
	return LuaToRESP(retVal), nil
}

func bytesTrimCRLF(b []byte) []byte {
	n := len(b)
	if n >= 2 && b[n-2] == '\r' && b[n-1] == '\n' {
		return b[:n-2]
	}
	return b
}
