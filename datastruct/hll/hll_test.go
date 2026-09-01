package hll

import (
	"fmt"
	"math"
	"testing"
)

func TestHLL_Accuracy(t *testing.T) {
	h := NewHLL()

	const numElements = 10000
	for i := 0; i < numElements; i++ {
		h.Add([]byte(fmt.Sprintf("elem_%d", i)))
	}

	est := h.Count()
	errorPercent := math.Abs(float64(est)-float64(numElements)) / float64(numElements) * 100

	t.Logf("Actual: %d, Estimated: %d, Error: %.2f%%", numElements, est, errorPercent)

	// Standard HLL theoretical error is ~0.81% (1.04 / sqrt(16384))
	// Allow within 2% margin for 10,000 items
	if errorPercent > 2.5 {
		t.Fatalf("HLL error too high: %.2f%%", errorPercent)
	}
}

func TestHLL_Merge(t *testing.T) {
	hA := NewHLL()
	hB := NewHLL()

	for i := 0; i < 5000; i++ {
		hA.Add([]byte(fmt.Sprintf("setA_%d", i)))
	}
	for i := 0; i < 5000; i++ {
		hB.Add([]byte(fmt.Sprintf("setB_%d", i)))
	}

	hA.Merge(hB)
	est := hA.Count()

	const totalElements = 10000
	errorPercent := math.Abs(float64(est)-float64(totalElements)) / float64(totalElements) * 100

	t.Logf("Merged Estimated: %d (Expected: %d, Error: %.2f%%)", est, totalElements, errorPercent)

	if errorPercent > 2.5 {
		t.Fatalf("HLL merged error too high: %.2f%%", errorPercent)
	}
}
