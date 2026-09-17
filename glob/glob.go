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

// matchOne reports whether p[j:] matches the single byte c, and how many
// pattern bytes that consumed. It handles a literal, '?', an escape and a
// character class.
func matchOne(p []byte, j int, c byte, fold bool) (bool, int) {
	switch p[j] {
	case '?':
		return true, 1

	case '[':
		k := j + 1
		neg := k < len(p) && p[k] == '^'
		if neg {
			k++
		}
		matched := false
		for k < len(p) && p[k] != ']' {
			if p[k] == '\\' && k+1 < len(p) {
				k++
				if p[k] == c {
					matched = true
				}
			} else if k+2 < len(p) && p[k+1] == '-' && p[k+2] != ']' {
				lo, hi, ch := p[k], p[k+2], c
				if lo > hi {
					lo, hi = hi, lo
				}
				if fold {
					lo, hi, ch = lower(lo), lower(hi), lower(ch)
				}
				if ch >= lo && ch <= hi {
					matched = true
				}
				k += 2
			} else {
				if fold {
					if lower(p[k]) == lower(c) {
						matched = true
					}
				} else if p[k] == c {
					matched = true
				}
			}
			k++
		}
		if k < len(p) && p[k] == ']' {
			k++
		}
		if neg {
			matched = !matched
		}
		return matched, k - j

	case '\\':
		if j+1 < len(p) {
			if fold {
				return lower(p[j+1]) == lower(c), 2
			}
			return p[j+1] == c, 2
		}
		return c == '\\', 1

	default:
		if fold {
			return lower(p[j]) == lower(c), 1
		}
		return p[j] == c, 1
	}
}

// matchBytes walks the pattern with a single backtrack point.
//
// The obvious recursive formulation retries every suffix at every '*', which is
// exponential: a pattern like "a*a*a*...*b" against a string of a's hangs the
// process, and KEYS takes its pattern straight from the client. Remembering
// only the most recent '*' and resuming from there keeps it linear in the
// product of the two lengths.
func matchBytes(p, s []byte, fold bool) bool {
	var (
		i, j      int
		star      = -1
		starMatch int
	)

	for i < len(s) {
		if j < len(p) && p[j] != '*' {
			if ok, width := matchOne(p, j, s[i], fold); ok {
				i++
				j += width
				continue
			}
		} else if j < len(p) {
			// Record this star and try matching the rest against s[i:].
			star = j
			starMatch = i
			j++
			continue
		}

		if star < 0 {
			return false
		}
		// Backtrack: let the remembered star absorb one more byte.
		j = star + 1
		starMatch++
		i = starMatch
	}

	for j < len(p) && p[j] == '*' {
		j++
	}
	return j == len(p)
}
