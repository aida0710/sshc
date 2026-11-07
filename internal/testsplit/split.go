// Package testsplit divides the Go packages among the CI jobs that run their tests, so that every
// job takes about as long. The race detector makes the tests of a few packages far slower than
// the rest (OpenSSH key encryption alone takes over a second a call), and one job per OS spent
// three to four minutes on them.
package testsplit

import (
	"fmt"
	"slices"
	"sort"
)

// Split returns the packages that part (counted from 1) of parts runs, sorted by name. It weighs
// each package by the seconds its tests took when they were last recorded, and hands the heaviest
// package left to the part with the least so far. A package with no recorded time weighs as much
// as the median of the recorded ones: a new package always runs somewhere, and the parts stay
// close to even until the times are recorded again.
func Split(packages []string, seconds map[string]float64, part, parts int) ([]string, error) {
	if parts < 1 || part < 1 || part > parts {
		return nil, fmt.Errorf("part %d of %d: want 1 <= part <= parts", part, parts)
	}
	weight := weigher(packages, seconds)
	ordered := slices.Clone(packages)
	slices.Sort(ordered)
	ordered = slices.Compact(ordered)
	// Heaviest first; equal weights in name order, so every job computes the same split.
	sort.SliceStable(ordered, func(left, right int) bool {
		return weight(ordered[left]) > weight(ordered[right])
	})

	totals := make([]float64, parts)
	assigned := make([][]string, parts)
	for _, name := range ordered {
		lightest := 0
		for index := range totals {
			if totals[index] < totals[lightest] {
				lightest = index
			}
		}
		totals[lightest] += weight(name)
		assigned[lightest] = append(assigned[lightest], name)
	}
	result := assigned[part-1]
	slices.Sort(result)
	return result, nil
}

// weigher returns the weight of a package: its recorded seconds, or the median of the recorded
// seconds of the given packages when it has none.
func weigher(packages []string, seconds map[string]float64) func(string) float64 {
	var recorded []float64
	for _, name := range packages {
		if value, ok := seconds[name]; ok {
			recorded = append(recorded, value)
		}
	}
	median := 1.0
	if len(recorded) > 0 {
		slices.Sort(recorded)
		median = recorded[len(recorded)/2]
	}
	return func(name string) float64 {
		if value, ok := seconds[name]; ok {
			return value
		}
		return median
	}
}
