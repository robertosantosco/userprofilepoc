package comparer

import (
	"time"
	"github.com/google/go-cmp/cmp"
)

func TimeWithinTolerance(toleranceMs int) cmp.Option {
	tolerance := time.Duration(toleranceMs) * time.Millisecond

	return cmp.Comparer(func(x, y time.Time) bool {
		diff := x.Sub(y)
		if diff < 0 {
			diff = -diff
		}
		return diff <= tolerance
	})
}