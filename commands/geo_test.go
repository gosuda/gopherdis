package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/gopherdis/db"
)

func TestGeoCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// 1. GEOADD Sicily 13.361389 38.115556 "Palermo" 15.087269 37.502669 "Catania" -> returns :2
	res := DefaultTable.Execute(ctx, [][]byte{
		[]byte("GEOADD"), []byte("Sicily"),
		[]byte("13.361389"), []byte("38.115556"), []byte("Palermo"),
		[]byte("15.087269"), []byte("37.502669"), []byte("Catania"),
	})
	if string(res) != ":2\r\n" {
		t.Fatalf("expected :2 on GEOADD, got %q", res)
	}

	// 2. GEODIST Sicily Palermo Catania km -> ~166.27 km
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("GEODIST"), []byte("Sicily"), []byte("Palermo"), []byte("Catania"), []byte("km"),
	})
	if !strings.Contains(string(res), "166.") {
		t.Fatalf("expected ~166.xx km from GEODIST, got %q", res)
	}

	// 3. GEOPOS Sicily Palermo Catania
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("GEOPOS"), []byte("Sicily"), []byte("Palermo"), []byte("Catania"),
	})
	if !strings.Contains(string(res), "13.36138") || !strings.Contains(string(res), "15.08726") {
		t.Fatalf("GEOPOS mismatch, got %q", res)
	}

	// 4. GEOHASH Sicily Palermo
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("GEOHASH"), []byte("Sicily"), []byte("Palermo"),
	})
	if !strings.HasPrefix(string(res), "*1\r\n") {
		t.Fatalf("expected 1-element array from GEOHASH, got %q", res)
	}

	// 5. GEORADIUS Sicily 15 37 200 km WITHDIST
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("GEORADIUS"), []byte("Sicily"), []byte("15"), []byte("37"), []byte("200"), []byte("km"), []byte("WITHDIST"),
	})
	if !strings.Contains(string(res), "Palermo") || !strings.Contains(string(res), "Catania") {
		t.Fatalf("expected both cities in GEORADIUS, got %q", res)
	}

	// 6. GEOSEARCH Sicily FROMMEMBER Palermo BYRADIUS 200 km WITHDIST
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("GEOSEARCH"), []byte("Sicily"), []byte("FROMMEMBER"), []byte("Palermo"),
		[]byte("BYRADIUS"), []byte("200"), []byte("km"), []byte("WITHDIST"),
	})
	if !strings.Contains(string(res), "Catania") {
		t.Fatalf("expected Catania in GEOSEARCH from Palermo, got %q", res)
	}

	// 7. Interoperability check: ZRANGE Sicily 0 -1 WITHSCORES
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("ZRANGE"), []byte("Sicily"), []byte("0"), []byte("-1"), []byte("WITHSCORES"),
	})
	if !strings.Contains(string(res), "Palermo") || !strings.Contains(string(res), "Catania") {
		t.Fatalf("expected ZSet interoperability, got %q", res)
	}
}
