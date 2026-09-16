// Package glob implements Redis' glob-style pattern matching.
package glob

// Match reports whether s matches the Redis glob pattern.
//
// This is a port of Redis' stringmatchlen: '*' matches any run including '/',
// '?' matches one byte, '[...]' is a class supporting '^' negation and 'a-z'
// ranges, and '\' escapes the next byte. path.Match is not a substitute because
// its '*' stops at '/', which silently changes the meaning of key and channel
// patterns that contain slashes.
func Match(pattern, s string) bool {
	return matchBytes([]byte(pattern), []byte(s), false)
}

// MatchFold is Match with ASCII case-insensitive comparison.
func MatchFold(pattern, s string) bool {
	return matchBytes([]byte(pattern), []byte(s), true)
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func matchBytes(p, s []byte, fold bool) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			// Collapse runs of '*' so "a**b" costs no more than "a*b".
			for len(p) > 1 && p[1] == '*' {
				p = p[1:]
			}
			if len(p) == 1 {
				return true // trailing star matches the rest
			}
			for i := 0; i <= len(s); i++ {
				if matchBytes(p[1:], s[i:], fold) {
					return true
				}
			}
			return false

		case '?':
			if len(s) == 0 {
				return false
			}
			s = s[1:]

		case '[':
			if len(s) == 0 {
				return false
			}
			p = p[1:]
			neg := len(p) > 0 && p[0] == '^'
			if neg {
				p = p[1:]
			}
			match := false
			for {
				if len(p) == 0 {
					// Unterminated class: Redis treats it as ending here.
					break
				}
				if p[0] == '\\' && len(p) >= 2 {
					p = p[1:]
					if p[0] == s[0] {
						match = true
					}
				} else if p[0] == ']' {
					break
				} else if len(p) >= 3 && p[1] == '-' && p[2] != ']' {
					start, end := p[0], p[2]
					if start > end {
						start, end = end, start
					}
					c := s[0]
					if fold {
						start, end, c = lower(start), lower(end), lower(c)
					}
					p = p[2:]
					if c >= start && c <= end {
						match = true
					}
				} else {
					if fold {
						if lower(p[0]) == lower(s[0]) {
							match = true
						}
					} else if p[0] == s[0] {
						match = true
					}
				}
				p = p[1:]
			}
			if neg {
				match = !match
			}
			if !match {
				return false
			}
			s = s[1:]

		case '\\':
			if len(p) >= 2 {
				p = p[1:]
			}
			fallthrough

		default:
			if len(s) == 0 {
				return false
			}
			if fold {
				if lower(p[0]) != lower(s[0]) {
					return false
				}
			} else if p[0] != s[0] {
				return false
			}
			s = s[1:]
		}

		p = p[1:]
		if len(s) == 0 {
			// Any remaining pattern must be all stars to still match.
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			break
		}
	}
	return len(p) == 0 && len(s) == 0
}
