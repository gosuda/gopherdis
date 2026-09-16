package commands

import "math"

// maxRandCount bounds how many elements one HRANDFIELD, SRANDMEMBER or
// ZRANDMEMBER call may be asked to produce.
//
// A negative count means "allow repeats, return exactly this many". Negating
// it without a check overflows at math.MinInt64 and the subsequent allocation
// takes the process down, so the count is range checked before use.
const maxRandCount = 1 << 24

// randCount normalizes a RANDMEMBER-style count argument. It reports the
// absolute number of elements to emit, whether repeats are allowed, and an
// error reply when the count is unusable.
func randCount(count int64) (n int, withRepeats bool, errReply []byte) {
	if count == math.MinInt64 {
		return 0, false, Error("value is out of range")
	}
	withRepeats = count < 0
	if withRepeats {
		count = -count
	}
	if count > maxRandCount {
		return 0, false, Error("value is out of range")
	}
	return int(count), withRepeats, nil
}
