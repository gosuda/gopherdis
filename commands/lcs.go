package commands

import (
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "lcs",
		Handler: lcsCommand,
		Arity:   -3,
		Flags:   FlagReadOnly,
	})
}

// lcsCommand computes the longest common subsequence of two string values.
//
// LEN reports only its length. IDX reports the ranges the match occupies in
// each input, walked back out of the same table, newest match first.
func lcsCommand(ctx *Context, argv [][]byte) []byte {
	keyA, keyB := string(argv[1]), string(argv[2])

	var wantLen, wantIdx, withMatchLen bool
	minMatchLen := 0
	for i := 3; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "LEN":
			wantLen = true
		case "IDX":
			wantIdx = true
		case "WITHMATCHLEN":
			withMatchLen = true
		case "MINMATCHLEN":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			n, err := strconv.Atoi(string(argv[i+1]))
			if err != nil {
				return Error("value is not an integer or out of range")
			}
			if n > 0 {
				minMatchLen = n
			}
			i++
		default:
			return Error("syntax error")
		}
	}
	if wantLen && wantIdx {
		return Error("If you want both the length and indexes, please just use IDX.")
	}

	a, errReply := lcsOperand(ctx, keyA)
	if errReply != nil {
		return errReply
	}
	b, errReply := lcsOperand(ctx, keyB)
	if errReply != nil {
		return errReply
	}

	// Classic edit-distance table: table[i][j] is the LCS length of a[:i], b[:j].
	rows, cols := len(a)+1, len(b)+1
	flat := make([]uint32, rows*cols)
	at := func(i, j int) uint32 { return flat[i*cols+j] }
	set := func(i, j int, v uint32) { flat[i*cols+j] = v }

	for i := 1; i < rows; i++ {
		for j := 1; j < cols; j++ {
			if a[i-1] == b[j-1] {
				set(i, j, at(i-1, j-1)+1)
			} else if at(i-1, j) >= at(i, j-1) {
				set(i, j, at(i-1, j))
			} else {
				set(i, j, at(i, j-1))
			}
		}
	}

	if wantLen {
		return Integer(int64(at(len(a), len(b))))
	}

	// Walk the table back. Equal bytes extend the current run; anything else
	// closes it and steps toward the larger neighbour.
	var result []byte
	type match struct{ aStart, aEnd, bStart, bEnd, length int }
	var matches []match

	i, j := len(a), len(b)
	runEndA, runEndB, runLen := 0, 0, 0
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			if runLen == 0 {
				runEndA, runEndB = i-1, j-1
			}
			runLen++
			result = append(result, a[i-1])
			i--
			j--
			continue
		}
		if runLen > 0 {
			matches = append(matches, match{i, runEndA, j, runEndB, runLen})
			runLen = 0
		}
		if at(i-1, j) > at(i, j-1) {
			i--
		} else {
			j--
		}
	}
	if runLen > 0 {
		matches = append(matches, match{i, runEndA, j, runEndB, runLen})
	}

	if !wantIdx {
		// The walk produced the subsequence backwards.
		for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
			result[l], result[r] = result[r], result[l]
		}
		return BulkString(result)
	}

	elems := make([][]byte, 0, len(matches))
	for _, m := range matches {
		if m.length < minMatchLen {
			continue
		}
		pair := [][]byte{
			Array([][]byte{Integer(int64(m.aStart)), Integer(int64(m.aEnd))}),
			Array([][]byte{Integer(int64(m.bStart)), Integer(int64(m.bEnd))}),
		}
		if withMatchLen {
			pair = append(pair, Integer(int64(m.length)))
		}
		elems = append(elems, Array(pair))
	}
	return MapReply(ctx, [][]byte{
		BulkString([]byte("matches")), Array(elems),
		BulkString([]byte("len")), Integer(int64(at(len(a), len(b)))),
	})
}

// lcsOperand reads a key's string value; a missing key reads as empty.
func lcsOperand(ctx *Context, key string) ([]byte, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return nil, nil
	}
	if obj.Type != object.OBJ_STRING {
		return nil, Error("The specified keys must contain string values")
	}
	return obj.Bytes(), nil
}
