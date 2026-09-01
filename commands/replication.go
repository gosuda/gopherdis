package commands

import (
	"strconv"
	"strings"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "replicaof",
		Handler: replicaofCommand,
		Arity:   3,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "slaveof",
		Handler: replicaofCommand,
		Arity:   3,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "role",
		Handler: roleCommand,
		Arity:   1,
		Flags:   FlagReadOnly | FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "replconf",
		Handler: replconfCommand,
		Arity:   -1,
		Flags:   FlagAdmin,
	})
}

func replicaofCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.Replication == nil {
		return Error("replication is disabled")
	}

	host := string(argv[1])
	portStr := string(argv[2])

	if strings.ToLower(host) == "no" && strings.ToLower(portStr) == "one" {
		ctx.Replication.ReplicaOf("no", 0)
		return OK()
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return Error("invalid port")
	}

	ctx.Replication.ReplicaOf(host, port)
	return OK()
}

func roleCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.Replication == nil {
		// Default master role
		return Array([][]byte{
			BulkString([]byte("master")),
			Integer(0),
			Array(nil),
		})
	}

	role := ctx.Replication.Role()
	return Array([][]byte{
		BulkString([]byte(role)),
		Integer(0),
		Array(nil),
	})
}

func replconfCommand(ctx *Context, argv [][]byte) []byte {
	return OK()
}
