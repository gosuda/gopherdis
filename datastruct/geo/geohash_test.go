package geo

import (
	"math"
	"testing"
)

func TestGeohash_EncodeDecode(t *testing.T) {
	// Palermo coordinates: 13.361389, 38.115556
	origLon := 13.361389
	origLat := 38.115556

	hash, err := Encode(origLon, origLat)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decLon, decLat := Decode(hash)
	if math.Abs(decLon-origLon) > 0.0001 || math.Abs(decLat-origLat) > 0.0001 {
		t.Fatalf("Decoded coordinates mismatch: (%f, %f) vs (%f, %f)", decLon, decLat, origLon, origLat)
	}

	base32 := EncodeBase32(hash)
	if len(base32) != 11 {
		t.Fatalf("expected 11 char base32, got %s", base32)
	}
}

func TestGeohash_HaversineDistance(t *testing.T) {
	// Palermo: 13.361389, 38.115556
	// Catania: 15.087269, 37.502669
	distMeters := HaversineDistance(13.361389, 38.115556, 15.087269, 37.502669)
	distKm, _ := ConvertDistance(distMeters, "km")

	// Expected ~ 166.27 km
	if math.Abs(distKm-166.27) > 1.0 {
		t.Fatalf("Distance between Palermo and Catania expected ~166.27km, got %f km", distKm)
	}
}
