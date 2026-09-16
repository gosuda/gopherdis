package glob

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "", true},
		{"*", "anything", true},
		{"h?llo", "hello", true},
		{"h?llo", "hllo", false},
		{"h*llo", "heeeello", true},
		{"h[ae]llo", "hallo", true},
		{"h[ae]llo", "hillo", false},
		{"h[^e]llo", "hallo", true},
		{"h[^e]llo", "hello", false},
		{"h[a-c]llo", "hbllo", true},
		{"h[a-c]llo", "hdllo", false},
		{"key:*", "key:1", true},
		{"key:*", "other", false},
		// path.Match would fail these: its '*' does not cross '/'.
		{"*", "a/b/c", true},
		{"news.*", "news.tech/ai", true},
		{"a*c", "a/b/c", true},
		{`h\*llo`, "h*llo", true},
		{`h\*llo`, "hello", false},
		{"", "", true},
		{"", "x", false},
		{"a*", "a", true},
		{"*a", "a", true},
		{"**b", "ab", true},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.s); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func TestMatchFold(t *testing.T) {
	if !MatchFold("HELLO", "hello") {
		t.Error("MatchFold should ignore ASCII case")
	}
	if Match("HELLO", "hello") {
		t.Error("Match should be case sensitive")
	}
}
