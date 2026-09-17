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

// matchBytes is a port of Redis' stringmatchlen_impl, including its
// skipLongerMatches bail-out.
//
// That flag is what bounds the recursion: once a '*' has consumed the whole
// remaining string without matching, every enclosing '*' stops trying too, so
// "a*a*a*...*b" cannot blow up exponentially. It also changes the answer.
// Matching "*?" repeated 50000 times against 50000 characters is a match by the
// plain definition of the syntax, and Redis reports no match. Reproducing that
// is the point: a drop-in replacement has to agree with Redis on what KEYS
// returns, and unit/keyspace asserts this exact case.
func matchBytes(p, s []byte, fold bool) bool {
	skipLongerMatches := false
	return matchImpl(p, s, fold, &skipLongerMatches, 0)
}

// maxNesting mirrors Redis' own limit on how deeply '*' may recurse. Past it
// Redis reports no match, which is why a pattern of 50000 stars finds nothing
// even against a string long enough to satisfy it.
const maxNesting = 1000

func matchImpl(p, s []byte, fold bool, skipLongerMatches *bool, nesting int) bool {
	if nesting > maxNesting {
		return false
	}

	for len(p) > 0 && len(s) > 0 {
		switch p[0] {
		case '*':
			for len(p) >= 2 && p[1] == '*' {
				p = p[1:]
			}
			if len(p) == 1 {
				return true // a trailing star matches the rest
			}
			for len(s) > 0 {
				if matchImpl(p[1:], s, fold, skipLongerMatches, nesting+1) {
					return true
				}
				if *skipLongerMatches {
					return false
				}
				s = s[1:]
			}
			// This star exhausted the string, so no longer prefix can help any
			// enclosing star either.
			*skipLongerMatches = true
			return false

		case '?':
			s = s[1:]

		case '[':
			ok, width := matchOne(p, 0, s[0], fold)
			if !ok {
				return false
			}
			p = p[width-1:]
			s = s[1:]

		case '\\':
			if len(p) >= 2 {
				p = p[1:]
			}
			fallthrough

		default:
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
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			break
		}
	}
	// An empty string still matches a pattern that is nothing but stars.
	if len(s) == 0 {
		for len(p) > 0 && p[0] == '*' {
			p = p[1:]
		}
	}
	return len(p) == 0 && len(s) == 0
}
