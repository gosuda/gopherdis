package stream

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidStreamID = errors.New("ERR The ID specified in XADD is equal or smaller than the target stream top item")
	ErrInvalidIDFormat = errors.New("ERR Invalid stream ID specified as stream command argument")
)

// StreamID represents the 128-bit monotonically increasing stream message ID (milliseconds-sequence).
type StreamID struct {
	Ms  uint64
	Seq uint64
}

// ZeroID represents the minimum possible ID (0-0).
var ZeroID = StreamID{Ms: 0, Seq: 0}

// MaxID represents the maximum possible ID.
var MaxID = StreamID{Ms: ^uint64(0), Seq: ^uint64(0)}

// String formats the ID as "<ms>-<seq>".
func (id StreamID) String() string {
	return fmt.Sprintf("%d-%d", id.Ms, id.Seq)
}

// Compare returns -1 if id < other, 0 if id == other, 1 if id > other.
func (id StreamID) Compare(other StreamID) int {
	if id.Ms < other.Ms {
		return -1
	} else if id.Ms > other.Ms {
		return 1
	}
	if id.Seq < other.Seq {
		return -1
	} else if id.Seq > other.Seq {
		return 1
	}
	return 0
}

// ParseID parses a stream ID string ("*", "<ms>-*", "<ms>-<seq>", "-", "+").
func ParseID(s string, lastID StreamID) (StreamID, error) {
	s = strings.TrimSpace(s)
	if s == "-" {
		return ZeroID, nil
	}
	if s == "+" {
		return MaxID, nil
	}

	if s == "*" {
		// Fully auto-generated ID: current time in ms, seq = 0 (or lastID.Seq + 1 if same ms)
		nowMs := uint64(time.Now().UnixMilli())
		if nowMs < lastID.Ms {
			nowMs = lastID.Ms
		}
		var seq uint64 = 0
		if nowMs == lastID.Ms {
			seq = lastID.Seq + 1
		}
		return StreamID{Ms: nowMs, Seq: seq}, nil
	}

	parts := strings.Split(s, "-")
	if len(parts) == 1 {
		// Only ms provided (e.g. for range query, defaults seq to 0)
		ms, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return ZeroID, ErrInvalidIDFormat
		}
		return StreamID{Ms: ms, Seq: 0}, nil
	} else if len(parts) == 2 {
		ms, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return ZeroID, ErrInvalidIDFormat
		}

		if parts[1] == "*" {
			// Auto-generate sequence part
			var seq uint64 = 0
			if ms == lastID.Ms {
				seq = lastID.Seq + 1
			}
			return StreamID{Ms: ms, Seq: seq}, nil
		}

		seq, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return ZeroID, ErrInvalidIDFormat
		}

		return StreamID{Ms: ms, Seq: seq}, nil
	}

	return ZeroID, ErrInvalidIDFormat
}
