package commands

import (
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/glob"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("hscan", hscanCommand, -3, FlagReadOnly)
	reg("sscan", sscanCommand, -3, FlagReadOnly)
	reg("zscan", zscanCommand, -3, FlagReadOnly)
}

// scanOpts holds the options shared by the collection scans.
type scanOpts struct {
	pattern  string
	noValues bool
}

func parseScanOpts(argv [][]byte, from int) (scanOpts, []byte) {
	var o scanOpts
	for i := from; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "MATCH":
			if i+1 >= len(argv) {
				return o, Error("syntax error")
			}
			o.pattern = string(argv[i+1])
			i++
		case "COUNT":
			if i+1 >= len(argv) {
				return o, Error("syntax error")
			}
			if n, err := strconv.Atoi(string(argv[i+1])); err != nil || n < 1 {
				return o, Error("syntax error")
			}
			i++
		case "NOVALUES":
			o.noValues = true
		default:
			return o, Error("syntax error")
		}
	}
	return o, nil
}

// scanReply pairs a cursor with the collected elements.
//
// These scans always return the whole collection and a zero cursor. SCAN only
// promises that an element present for the entire iteration is returned at
// least once, and returning everything in one call satisfies that; COUNT is
// advisory, exactly as it is for the keyspace SCAN.
func scanReply(elements [][]byte) []byte {
	return Array([][]byte{
		BulkString([]byte("0")),
		Array(elements),
	})
}

func scanCursorOK(argv [][]byte) []byte {
	if _, err := strconv.ParseUint(string(argv[2]), 10, 64); err != nil {
		return Error("invalid cursor")
	}
	return nil
}

func hscanCommand(ctx *Context, argv [][]byte) []byte {
	if errReply := scanCursorOK(argv); errReply != nil {
		return errReply
	}
	opts, errReply := parseScanOpts(argv, 3)
	if errReply != nil {
		return errReply
	}

	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil {
		return scanReply(nil)
	}

	var out [][]byte
	d.ForEach(func(f string, v []byte) {
		if opts.pattern != "" && !glob.Match(opts.pattern, f) {
			return
		}
		out = append(out, BulkString([]byte(f)))
		if !opts.noValues {
			out = append(out, BulkString(v))
		}
	})
	return scanReply(out)
}

func sscanCommand(ctx *Context, argv [][]byte) []byte {
	if errReply := scanCursorOK(argv); errReply != nil {
		return errReply
	}
	opts, errReply := parseScanOpts(argv, 3)
	if errReply != nil {
		return errReply
	}

	s, errReply := getSetView(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return scanReply(nil)
	}

	var out [][]byte
	for _, m := range s.Members() {
		if opts.pattern != "" && !glob.Match(opts.pattern, m) {
			continue
		}
		out = append(out, BulkString([]byte(m)))
	}
	return scanReply(out)
}

func zscanCommand(ctx *Context, argv [][]byte) []byte {
	if errReply := scanCursorOK(argv); errReply != nil {
		return errReply
	}
	opts, errReply := parseScanOpts(argv, 3)
	if errReply != nil {
		return errReply
	}

	z, errReply := getZSetForRead(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}

	var out [][]byte
	for _, el := range z.elements() {
		if opts.pattern != "" && !glob.Match(opts.pattern, el.Member) {
			continue
		}
		out = append(out, BulkString([]byte(el.Member)), BulkString([]byte(formatFloat(el.Score))))
	}
	return scanReply(out)
}
