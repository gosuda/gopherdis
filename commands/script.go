package commands

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/scripting"
)

var defaultScriptEngine = scripting.NewEngine()

func init() {
	DefaultTable.Register(&Command{
		Name:    "eval",
		Handler: evalCommand,
		Arity:   -3,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "evalsha",
		Handler: evalshaCommand,
		Arity:   -3,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "script",
		Handler: scriptCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "function",
		Handler: functionCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
}

// functionCommand answers the FUNCTION container for a server with no function
// engine. The read-only subcommands report an empty library set, which is the
// truthful answer here; LOAD and RESTORE report that the feature is absent
// rather than pretending to succeed.
func functionCommand(ctx *Context, argv [][]byte) []byte {
	switch strings.ToUpper(string(argv[1])) {
	case "FLUSH":
		// Nothing is ever loaded, so flushing is a no-op rather than an error.
		return OK()
	case "LIST":
		return Array(nil)
	case "DUMP":
		return BulkString(nil)
	case "STATS":
		// running_script (nil) + engines (empty map), as Redis reports when idle.
		return []byte("*4\r\n$14\r\nrunning_script\r\n$-1\r\n$7\r\nengines\r\n*0\r\n")
	case "LOAD", "RESTORE":
		return Error("function engine is not supported")
	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", string(argv[1])))
	}
}

func getScriptEngine(ctx *Context) *scripting.Engine {
	if ctx != nil && ctx.Scripting != nil {
		return ctx.Scripting
	}
	return defaultScriptEngine
}

func evalCommand(ctx *Context, argv [][]byte) []byte {
	script := string(argv[1])
	numKeys, err := strconv.Atoi(string(argv[2]))
	if err != nil || numKeys < 0 {
		return Error("value is not an integer or out of range")
	}

	if len(argv) < 3+numKeys {
		return Error("wrong number of arguments for 'eval' command")
	}

	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = string(argv[3+i])
	}

	argsRaw := argv[3+numKeys:]
	args := make([]string, len(argsRaw))
	for i, a := range argsRaw {
		args[i] = string(a)
	}

	if ctx != nil && ctx.DB != nil {
		ctx.DB.BeginTx()
		defer ctx.DB.EndTx()
		ctx.InTxExecution = true
		defer func() { ctx.InTxExecution = false }()
	}

	eng := getScriptEngine(ctx)
	exec := func(cmdArgv [][]byte) []byte {
		return DefaultTable.Execute(ctx, cmdArgv)
	}

	res, err := eng.Eval(script, keys, args, exec)
	if err != nil {
		return Error(err.Error())
	}

	return res
}

func evalshaCommand(ctx *Context, argv [][]byte) []byte {
	hash := strings.ToLower(string(argv[1]))
	numKeys, err := strconv.Atoi(string(argv[2]))
	if err != nil || numKeys < 0 {
		return Error("value is not an integer or out of range")
	}

	if len(argv) < 3+numKeys {
		return Error("wrong number of arguments for 'evalsha' command")
	}

	if ctx != nil && ctx.DB != nil {
		ctx.DB.BeginTx()
		defer ctx.DB.EndTx()
		ctx.InTxExecution = true
		defer func() { ctx.InTxExecution = false }()
	}

	eng := getScriptEngine(ctx)
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = string(argv[3+i])
	}

	argsRaw := argv[3+numKeys:]
	args := make([]string, len(argsRaw))
	for i, a := range argsRaw {
		args[i] = string(a)
	}

	exec := func(cmdArgv [][]byte) []byte {
		return DefaultTable.Execute(ctx, cmdArgv)
	}

	res, err := eng.EvalSHA(hash, keys, args, exec)
	if err != nil {
		if errors.Is(err, scripting.ErrNoSuchScript) {
			return []byte("-NOSCRIPT No matching script. Please use EVAL.\r\n")
		}
		return Error(err.Error())
	}

	return res
}

func scriptCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))
	eng := getScriptEngine(ctx)

	switch subCmd {
	case "LOAD":
		if len(argv) != 3 {
			return Error("wrong number of arguments for 'script load' command")
		}
		script := string(argv[2])
		hash := eng.LoadScript(script)
		return BulkString([]byte(hash))

	case "EXISTS":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'script exists' command")
		}
		hashes := make([]string, len(argv)-2)
		for i := 2; i < len(argv); i++ {
			hashes[i-2] = strings.ToLower(string(argv[i]))
		}
		exists := eng.ExistsScripts(hashes)
		replies := make([][]byte, len(exists))
		for i, e := range exists {
			if e {
				replies[i] = Integer(1)
			} else {
				replies[i] = Integer(0)
			}
		}
		return Array(replies)

	case "FLUSH":
		eng.FlushScripts()
		return OK()

	case "KILL":
		// There is no per-script kill switch; scripts are bounded by
		// scripting.ScriptTimeout instead. Reply the way Redis does when nothing
		// is killable rather than returning a +OK that did nothing.
		return Error("NOTBUSY No scripts in execution right now.")

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}
