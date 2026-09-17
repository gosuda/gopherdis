package glob

import (
	"strings"
	"testing"
	"time"
)

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

// TestPathologicalPatternsTerminate guards against catastrophic backtracking.
//
// The naive recursive matcher retries every suffix at every '*', so these
// patterns take exponential time. KEYS and the Pub/Sub pattern router both take
// their pattern straight from the client, so that is a denial of service, not
// just a slow query.
func TestPathologicalPatternsTerminate(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{strings.Repeat("a*", 20) + "b", strings.Repeat("a", 40), false},
		{strings.Repeat("a*", 20) + "b", strings.Repeat("a", 40) + "b", true},
		// 200 stars stays under Redis' nesting limit, so this one matches.
		{strings.Repeat("*?", 200), strings.Repeat("a", 400), true},
		{strings.Repeat("*", 100) + "z", strings.Repeat("a", 200), false},
	}
	for _, c := range cases {
		done := make(chan bool, 1)
		go func() { done <- Match(c.pattern, c.s) }()
		select {
		case got := <-done:
			if got != c.want {
				t.Errorf("Match(%d-byte pattern, %d-byte string) = %v, want %v",
					len(c.pattern), len(c.s), got, c.want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Match did not terminate for a %d-byte pattern", len(c.pattern))
		}
	}
}

// TestLongPatternIsLinear is the shape unit/keyspace exercises directly.
//
// 50000 stars exceeds Redis' nesting limit, so Redis reports no match even
// though the pattern would otherwise be satisfied. A drop-in replacement has to
// give the same answer.
func TestLongPatternIsLinear(t *testing.T) {
	start := time.Now()
	if Match(strings.Repeat("*?", 50000), strings.Repeat("a", 50000)) != false {
		t.Error("expected Redis' no-match answer past the nesting limit")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("matching took %v, which is not linear", d)
	}
}
