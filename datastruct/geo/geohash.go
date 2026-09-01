package geo

import (
	"errors"
	"math"
	"strings"
)

const (
	GEO_LAT_MIN = -85.05112878
	GEO_LAT_MAX = 85.05112878
	GEO_LON_MIN = -180.0
	GEO_LON_MAX = 180.0
	GEO_STEP_BITS = 26 // 26 bits lat + 26 bits lon = 52 bits
	EARTH_RADIUS_M = 6372797.56085
)

var (
	ErrInvalidCoordinates = errors.New("ERR coordinates are out of range")
	ErrInvalidUnit        = errors.New("ERR unsupported unit provided. please use m, km, ft, mi")
)

// Encode converts longitude and latitude to a 52-bit Geohash integer.
func Encode(lon, lat float64) (uint64, error) {
	if lon < GEO_LON_MIN || lon > GEO_LON_MAX || lat < GEO_LAT_MIN || lat > GEO_LAT_MAX {
		return 0, ErrInvalidCoordinates
	}

	latMin, latMax := GEO_LAT_MIN, GEO_LAT_MAX
	lonMin, lonMax := GEO_LON_MIN, GEO_LON_MAX

	var hash uint64 = 0

	for i := 0; i < GEO_STEP_BITS; i++ {
		// Longitude bit (even bit in interleaved hash)
		lonMid := (lonMin + lonMax) / 2
		if lon >= lonMid {
			hash = (hash << 1) | 1
			lonMin = lonMid
		} else {
			hash = (hash << 1) | 0
			lonMax = lonMid
		}

		// Latitude bit (odd bit in interleaved hash)
		latMid := (latMin + latMax) / 2
		if lat >= latMid {
			hash = (hash << 1) | 1
			latMin = latMid
		} else {
			hash = (hash << 1) | 0
			latMax = latMid
		}
	}

	return hash, nil
}

// Decode recovers longitude and latitude from a 52-bit Geohash integer.
func Decode(hash uint64) (lon, lat float64) {
	latMin, latMax := GEO_LAT_MIN, GEO_LAT_MAX
	lonMin, lonMax := GEO_LON_MIN, GEO_LON_MAX

	for i := GEO_STEP_BITS - 1; i >= 0; i-- {
		// Latitude bit
		latBit := (hash >> uint(i*2)) & 1
		// Longitude bit
		lonBit := (hash >> uint(i*2+1)) & 1

		lonMid := (lonMin + lonMax) / 2
		if lonBit == 1 {
			lonMin = lonMid
		} else {
			lonMax = lonMid
		}

		latMid := (latMin + latMax) / 2
		if latBit == 1 {
			latMin = latMid
		} else {
			latMax = latMid
		}
	}

	return (lonMin + lonMax) / 2, (latMin + latMax) / 2
}

// HaversineDistance calculates the great-circle distance between two points in meters.
func HaversineDistance(lon1, lat1, lon2, lat2 float64) float64 {
	radLat1 := lat1 * math.Pi / 180.0
	radLat2 := lat2 * math.Pi / 180.0
	deltaLat := (lat2 - lat1) * math.Pi / 180.0
	deltaLon := (lon2 - lon1) * math.Pi / 180.0

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(radLat1)*math.Cos(radLat2)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return EARTH_RADIUS_M * c
}

// ConvertDistance converts meters to the specified unit (m, km, ft, mi).
func ConvertDistance(meters float64, unit string) (float64, error) {
	switch strings.ToLower(unit) {
	case "m":
		return meters, nil
	case "km":
		return meters / 1000.0, nil
	case "mi":
		return meters / 1609.344, nil
	case "ft":
		return meters / 0.3048, nil
	default:
		return 0, ErrInvalidUnit
	}
}

// ConvertToMeters converts distance in specified unit to meters.
func ConvertToMeters(dist float64, unit string) (float64, error) {
	switch strings.ToLower(unit) {
	case "m":
		return dist, nil
	case "km":
		return dist * 1000.0, nil
	case "mi":
		return dist * 1609.344, nil
	case "ft":
		return dist * 0.3048, nil
	default:
		return 0, ErrInvalidUnit
	}
}

const Base32Chars = "0123456789bcdefghjkmnpqrstuvwxyz"

// EncodeBase32 formats 52-bit Geohash as a standard 11-character base32 string matching Redis.
func EncodeBase32(hash uint64) string {
	lon, lat := Decode(hash)
	wLatMin, wLatMax := -90.0, 90.0
	wLonMin, wLonMax := -180.0, 180.0

	var buf [11]byte
	isLon := true

	for i := 0; i < 10; i++ {
		var charBits byte = 0
		for b := 0; b < 5; b++ {
			charBits <<= 1
			if isLon {
				mid := (wLonMin + wLonMax) / 2
				if lon >= mid {
					charBits |= 1
					wLonMin = mid
				} else {
					wLonMax = mid
				}
			} else {
				mid := (wLatMin + wLatMax) / 2
				if lat >= mid {
					charBits |= 1
					wLatMin = mid
				} else {
					wLatMax = mid
				}
			}
			isLon = !isLon
		}
		buf[i] = Base32Chars[charBits]
	}
	buf[10] = '0'
	return string(buf[:])
}

// BoundingBox calculates min and max lat/lon given a center point and radius in meters.
func BoundingBox(lon, lat float64, radiusMeters float64) (minLon, minLat, maxLon, maxLat float64) {
	deltaLat := (radiusMeters / EARTH_RADIUS_M) * (180.0 / math.Pi)
	minLat = math.Max(lat-deltaLat, GEO_LAT_MIN)
	maxLat = math.Min(lat+deltaLat, GEO_LAT_MAX)

	radLat := lat * math.Pi / 180.0
	cosLat := math.Cos(radLat)
	if cosLat < 1e-6 {
		cosLat = 1e-6
	}
	deltaLon := (radiusMeters / (EARTH_RADIUS_M * cosLat)) * (180.0 / math.Pi)
	minLon = math.Max(lon-deltaLon, GEO_LON_MIN)
	maxLon = math.Min(lon+deltaLon, GEO_LON_MAX)

	return minLon, minLat, maxLon, maxLat
}
