package util

import (
	"math"
)

func ComputeQuorum(n int32) int32 {
	return int32(math.Floor(float64(n/2) + 1))
}
