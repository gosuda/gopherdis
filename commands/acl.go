package commands

import (
	"fmt"
	"strings"

	"github.com/gosuda/nedis/acl"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "auth",
		Handler: authCommand,
		Arity:   -2,
		Flags:   FlagAdmin | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "acl",
		Handler: aclCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
}

func authCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.ACL == nil {
		return OK()
	}

	username := "default"
	password := ""

	if len(argv) == 2 {
		password = string(argv[1])
	} else if len(argv) >= 3 {
		username = string(argv[1])
		password = string(argv[2])
	}

	user, err := ctx.ACL.Auth(username, password)
	if err != nil {
		return []byte("-WRONGPASS invalid username-password pair or user is disabled\r\n")
	}

	ctx.User = user
	return OK()
}

func aclCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.ACL == nil {
		ctx.ACL = acl.NewManager()
	}

	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "WHOAMI":
		if ctx.User != nil {
			return BulkString([]byte(ctx.User.Name))
		}
		return BulkString([]byte("default"))

	case "USERS":
		users := ctx.ACL.ListUsers()
		replies := make([][]byte, len(users))
		for i, u := range users {
			replies[i] = BulkString([]byte(u))
		}
		return Array(replies)

	case "LIST":
		users := ctx.ACL.ListUsers()
		replies := make([][]byte, len(users))
		for i, name := range users {
			u := ctx.ACL.GetUser(name)
			status := "off"
			if u.Enabled {
				status = "on"
			}
			line := fmt.Sprintf("user %s %s ~* &* +@all", u.Name, status)
			replies[i] = BulkString([]byte(line))
		}
		return Array(replies)

	case "SETUSER":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'acl setuser' command")
		}
		username := string(argv[2])
		user := ctx.ACL.GetOrCreateUser(username)

		for i := 3; i < len(argv); i++ {
			rule := string(argv[i])
			if err := user.ApplyRule(rule); err != nil {
				return Error(fmt.Sprintf("ERR %v", err))
			}
		}
		return OK()

	case "GETUSER":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'acl getuser' command")
		}
		username := string(argv[2])
		user := ctx.ACL.GetUser(username)
		if user == nil {
			return NullArray()
		}

		status := "off"
		if user.Enabled {
			status = "on"
		}

		return Array([][]byte{
			BulkString([]byte("flags")),
			Array([][]byte{BulkString([]byte(status))}),
			BulkString([]byte("passwords")),
			Array(nil),
			BulkString([]byte("commands")),
			BulkString([]byte("+@all")),
			BulkString([]byte("keys")),
			BulkString([]byte("~*")),
			BulkString([]byte("channels")),
			BulkString([]byte("&*")),
		})

	case "DELUSER":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'acl deluser' command")
		}
		var deletedCount int64 = 0
		for i := 2; i < len(argv); i++ {
			name := string(argv[i])
			if ctx.ACL.DelUser(name) {
				deletedCount++
			}
		}
		return Integer(deletedCount)

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}
