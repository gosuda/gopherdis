package scripting

import (
	"bytes"
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

// RESPToLua converts a raw RESP binary response slice into corresponding Lua values.
func RESPToLua(L *lua.LState, resp []byte) lua.LValue {
	if len(resp) == 0 {
		return lua.LNil
	}

	switch resp[0] {
	case '+': // Simple String -> string (or status table)
		line := bytes.TrimSuffix(resp[1:], []byte("\r\n"))
		tbl := L.NewTable()
		tbl.RawSetString("ok", lua.LString(string(line)))
		return tbl

	case '-': // Error -> {err = "..."}
		line := bytes.TrimSuffix(resp[1:], []byte("\r\n"))
		tbl := L.NewTable()
		tbl.RawSetString("err", lua.LString(string(line)))
		return tbl

	case ':': // Integer -> number
		line := bytes.TrimSuffix(resp[1:], []byte("\r\n"))
		num, _ := strconv.ParseInt(string(line), 10, 64)
		return lua.LNumber(num)

	case '$': // Bulk String -> string or false/nil if -1
		idx := bytes.Index(resp, []byte("\r\n"))
		if idx == -1 {
			return lua.LNil
		}
		length, _ := strconv.Atoi(string(resp[1:idx]))
		if length == -1 {
			return lua.LFalse // Redis Lua converts null bulk string to false
		}
		payload := resp[idx+2 : idx+2+length]
		return lua.LString(string(payload))

	case '*': // Array -> table
		idx := bytes.Index(resp, []byte("\r\n"))
		if idx == -1 {
			return lua.LNil
		}
		count, _ := strconv.Atoi(string(resp[1:idx]))
		if count == -1 {
			return lua.LFalse
		}

		tbl := L.NewTable()
		rem := resp[idx+2:]
		for i := 1; i <= count; i++ {
			elem, nextRem := parseNextRESP(rem)
			tbl.RawSetInt(i, RESPToLua(L, elem))
			rem = nextRem
		}
		return tbl

	default:
		return lua.LString(string(resp))
	}
}

func parseNextRESP(data []byte) (elem []byte, rem []byte) {
	if len(data) == 0 {
		return nil, nil
	}
	switch data[0] {
	case '+', '-', ':':
		idx := bytes.Index(data, []byte("\r\n"))
		if idx == -1 {
			return data, nil
		}
		return data[:idx+2], data[idx+2:]
	case '$':
		idx := bytes.Index(data, []byte("\r\n"))
		if idx == -1 {
			return data, nil
		}
		length, _ := strconv.Atoi(string(data[1:idx]))
		if length == -1 {
			return data[:idx+2], data[idx+2:]
		}
		end := idx + 2 + length + 2
		if end > len(data) {
			return data, nil
		}
		return data[:end], data[end:]
	case '*':
		idx := bytes.Index(data, []byte("\r\n"))
		if idx == -1 {
			return data, nil
		}
		count, _ := strconv.Atoi(string(data[1:idx]))
		if count <= 0 {
			return data[:idx+2], data[idx+2:]
		}
		cur := data[idx+2:]
		for i := 0; i < count; i++ {
			_, next := parseNextRESP(cur)
			cur = next
		}
		consumed := len(data) - len(cur)
		return data[:consumed], cur
	default:
		return data, nil
	}
}

// LuaToRESP converts Lua return values back into standard RESP binary replies.
func LuaToRESP(val lua.LValue) []byte {
	if val == nil || val.Type() == lua.LTNil {
		return []byte("$-1\r\n")
	}

	switch v := val.(type) {
	case lua.LBool:
		if bool(v) {
			return []byte(":1\r\n")
		}
		return []byte("$-1\r\n")

	case lua.LNumber:
		return []byte(":" + strconv.FormatInt(int64(v), 10) + "\r\n")

	case lua.LString:
		s := string(v)
		return []byte("$" + strconv.Itoa(len(s)) + "\r\n" + s + "\r\n")

	case *lua.LTable:
		// Check for error table: {err = "..."}
		if errVal := v.RawGetString("err"); errVal.Type() == lua.LTString {
			return []byte("-" + errVal.String() + "\r\n")
		}
		// Check for status table: {ok = "..."}
		if okVal := v.RawGetString("ok"); okVal.Type() == lua.LTString {
			return []byte("+" + okVal.String() + "\r\n")
		}

		// Sequential array table
		n := v.Len()
		var buf bytes.Buffer
		buf.WriteString("*" + strconv.Itoa(n) + "\r\n")
		for i := 1; i <= n; i++ {
			elem := v.RawGetInt(i)
			buf.Write(LuaToRESP(elem))
		}
		return buf.Bytes()

	default:
		s := val.String()
		return []byte("$" + strconv.Itoa(len(s)) + "\r\n" + s + "\r\n")
	}
}
