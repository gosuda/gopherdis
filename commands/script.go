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
		return OK()

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}
