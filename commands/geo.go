package commands

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gosuda/nedis/datastruct/geo"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "geoadd",
		Handler: geoaddCommand,
		Arity:   -5,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "geodist",
		Handler: geodistCommand,
		Arity:   -4,
		Flags:   FlagReadOnly | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "geopos",
		Handler: geoposCommand,
		Arity:   -3,
		Flags:   FlagReadOnly | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "geohash",
		Handler: geohashCommand,
		Arity:   -3,
		Flags:   FlagReadOnly | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "georadius",
		Handler: georadiusCommand,
		Arity:   -6,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "georadiusbymember",
		Handler: georadiusbymemberCommand,
		Arity:   -5,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "geosearch",
		Handler: geosearchCommand,
		Arity:   -7,
		Flags:   FlagReadOnly,
	})
}

func geoaddCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	idx := 2
	nx := false
	xx := false
	ch := false

	// Parse flags (NX, XX, CH)
	for idx < len(argv) {
		opt := strings.ToUpper(string(argv[idx]))
		if opt == "NX" {
			nx = true
			idx++
		} else if opt == "XX" {
			xx = true
			idx++
		} else if opt == "CH" {
			ch = true
			idx++
		} else {
			break
		}
	}

	rem := argv[idx:]
	if len(rem) == 0 || len(rem)%3 != 0 {
		return Error("syntax error in GEOADD")
	}

	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	var addedCount int64 = 0
	var changedCount int64 = 0

	for i := 0; i < len(rem)/3; i++ {
		lonStr := string(rem[i*3])
		latStr := string(rem[i*3+1])
		member := string(rem[i*3+2])

		lon, err1 := strconv.ParseFloat(lonStr, 64)
		lat, err2 := strconv.ParseFloat(latStr, 64)
		if err1 != nil || err2 != nil {
			return Error("ERR value is not a valid float")
		}

		hash, err := geo.Encode(lon, lat)
		if err != nil {
			return Error(err.Error())
		}

		score := float64(hash)
		oldScore, exists := zs.Score(member)

		if exists && nx {
			continue
		}
		if !exists && xx {
			continue
		}

		added, updated := zs.Add(member, score)
		if added {
			addedCount++
			changedCount++
		} else if updated && oldScore != score {
			changedCount++
		}
	}

	if ch {
		return Integer(changedCount)
	}
	return Integer(addedCount)
}

func geodistCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	member1 := string(argv[2])
	member2 := string(argv[3])
	unit := "m"
	if len(argv) >= 5 {
		unit = string(argv[4])
	}

	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}
	if zs == nil || zs.Len() == 0 {
		return NullBulkString()
	}

	s1, exists1 := zs.Score(member1)
	s2, exists2 := zs.Score(member2)
	if !exists1 || !exists2 {
		return NullBulkString()
	}

	lon1, lat1 := geo.Decode(uint64(s1))
	lon2, lat2 := geo.Decode(uint64(s2))

	distMeters := geo.HaversineDistance(lon1, lat1, lon2, lat2)
	dist, err := geo.ConvertDistance(distMeters, unit)
	if err != nil {
		return Error(err.Error())
	}

	return BulkString([]byte(fmt.Sprintf("%.4f", dist)))
}

func geoposCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	members := argv[2:]
	results := make([][]byte, len(members))

	for i, m := range members {
		if zs == nil {
			results[i] = NullArray()
			continue
		}
		score, exists := zs.Score(string(m))
		if !exists {
			results[i] = NullArray()
			continue
		}
		lon, lat := geo.Decode(uint64(score))
		results[i] = Array([][]byte{
			BulkString([]byte(fmt.Sprintf("%.6f", lon))),
			BulkString([]byte(fmt.Sprintf("%.6f", lat))),
		})
	}

	return Array(results)
}

func geohashCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	members := argv[2:]
	results := make([][]byte, len(members))

	for i, m := range members {
		if zs == nil {
			results[i] = NullBulkString()
			continue
		}
		score, exists := zs.Score(string(m))
		if !exists {
			results[i] = NullBulkString()
			continue
		}
		b32 := geo.EncodeBase32(uint64(score))
		results[i] = BulkString([]byte(b32))
	}

	return Array(results)
}

type geoMatch struct {
	member   string
	dist     float64
	hash     uint64
	lon, lat float64
}

func georadiusCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	lon, err1 := strconv.ParseFloat(string(argv[2]), 64)
	lat, err2 := strconv.ParseFloat(string(argv[3]), 64)
	radius, err3 := strconv.ParseFloat(string(argv[4]), 64)
	unit := string(argv[5])

	if err1 != nil || err2 != nil || err3 != nil {
		return Error("ERR value is not a valid float")
	}

	return performGeoRadius(ctx, key, lon, lat, radius, unit, argv[6:])
}

func georadiusbymemberCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	member := string(argv[2])
	radius, err := strconv.ParseFloat(string(argv[3]), 64)
	if err != nil {
		return Error("ERR value is not a valid float")
	}
	unit := string(argv[4])

	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}
	if zs == nil || zs.Len() == 0 {
		return Array(nil)
	}

	score, exists := zs.Score(member)
	if !exists {
		return Error("ERR could not decode requested zset member")
	}

	lon, lat := geo.Decode(uint64(score))
	return performGeoRadius(ctx, key, lon, lat, radius, unit, argv[5:])
}

func geosearchCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	idx := 2
	var lon, lat float64
	var hasCoord bool

	radius := float64(0)
	unit := "m"
	var options [][]byte

	for idx < len(argv) {
		opt := strings.ToUpper(string(argv[idx]))
		if opt == "FROMMEMBER" {
			idx++
			member := string(argv[idx])
			idx++
			zs, _, errReply := getOrCreateZSet(ctx, key)
			if errReply != nil {
				return errReply
			}
			if zs == nil || zs.Len() == 0 {
				return Array(nil)
			}
			score, exists := zs.Score(member)
			if !exists {
				return Error("ERR could not decode requested zset member")
			}
			lon, lat = geo.Decode(uint64(score))
			hasCoord = true
		} else if opt == "FROMLONLAT" {
			idx++
			lo, err1 := strconv.ParseFloat(string(argv[idx]), 64)
			idx++
			la, err2 := strconv.ParseFloat(string(argv[idx]), 64)
			idx++
			if err1 != nil || err2 != nil {
				return Error("ERR value is not a valid float")
			}
			lon, lat = lo, la
			hasCoord = true
		} else if opt == "BYRADIUS" {
			idx++
			r, err := strconv.ParseFloat(string(argv[idx]), 64)
			idx++
			u := string(argv[idx])
			idx++
			if err != nil {
				return Error("ERR value is not a valid float")
			}
			radius = r
			unit = u
		} else {
			options = append(options, argv[idx])
			idx++
		}
	}

	if !hasCoord || radius <= 0 {
		return Error("syntax error in GEOSEARCH")
	}

	return performGeoRadius(ctx, key, lon, lat, radius, unit, options)
}

func performGeoRadius(ctx *Context, key string, lon, lat, radius float64, unit string, options [][]byte) []byte {
	radiusMeters, err := geo.ConvertToMeters(radius, unit)
	if err != nil {
		return Error(err.Error())
	}

	withCoord := false
	withDist := false
	withHash := false
	sortAsc := false
	sortDesc := false
	count := 0

	idx := 0
	for idx < len(options) {
		opt := strings.ToUpper(string(options[idx]))
		if opt == "WITHCOORD" {
			withCoord = true
			idx++
		} else if opt == "WITHDIST" {
			withDist = true
			idx++
		} else if opt == "WITHHASH" {
			withHash = true
			idx++
		} else if opt == "ASC" {
			sortAsc = true
			idx++
		} else if opt == "DESC" {
			sortDesc = true
			idx++
		} else if opt == "COUNT" {
			idx++
			if idx < len(options) {
				c, err := strconv.Atoi(string(options[idx]))
				if err == nil && c > 0 {
					count = c
				}
				idx++
			}
		} else {
			idx++
		}
	}

	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}
	if zs == nil || zs.Len() == 0 {
		return Array(nil)
	}

	minLon, minLat, maxLon, maxLat := geo.BoundingBox(lon, lat, radiusMeters)

	var matches []geoMatch

	// Scan all members in ZSet
	elements := zs.Range(0, -1, false)
	for _, el := range elements {
		memberHash := uint64(el.Score)
		mLon, mLat := geo.Decode(memberHash)

		if mLon < minLon || mLon > maxLon || mLat < minLat || mLat > maxLat {
			continue
		}

		distMeters := geo.HaversineDistance(lon, lat, mLon, mLat)
		if distMeters <= radiusMeters {
			distUnit, _ := geo.ConvertDistance(distMeters, unit)
			matches = append(matches, geoMatch{
				member: el.Member,
				dist:   distUnit,
				hash:   memberHash,
				lon:    mLon,
				lat:    mLat,
			})
		}
	}

	if sortAsc {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].dist < matches[j].dist
		})
	} else if sortDesc {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].dist > matches[j].dist
		})
	}

	if count > 0 && len(matches) > count {
		matches = matches[:count]
	}

	results := make([][]byte, len(matches))
	for i, m := range matches {
		if !withCoord && !withDist && !withHash {
			results[i] = BulkString([]byte(m.member))
		} else {
			item := [][]byte{BulkString([]byte(m.member))}
			if withDist {
				item = append(item, BulkString([]byte(fmt.Sprintf("%.4f", m.dist))))
			}
			if withHash {
				item = append(item, Integer(int64(m.hash)))
			}
			if withCoord {
				item = append(item, Array([][]byte{
					BulkString([]byte(fmt.Sprintf("%.6f", m.lon))),
					BulkString([]byte(fmt.Sprintf("%.6f", m.lat))),
				}))
			}
			results[i] = Array(item)
		}
	}

	return Array(results)
}
