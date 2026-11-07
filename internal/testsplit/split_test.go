package testsplit

import (
	"slices"
	"testing"
)

func splitAll(t *testing.T, packages []string, seconds map[string]float64, parts int) [][]string {
	t.Helper()
	result := make([][]string, parts)
	for part := 1; part <= parts; part++ {
		names, err := Split(packages, seconds, part, parts)
		if err != nil {
			t.Fatalf("Split(part %d of %d) error = %v", part, parts, err)
		}
		result[part-1] = names
	}
	return result
}

func TestEveryPackageRunsInExactlyOnePart(t *testing.T) {
	packages := []string{"a", "b", "c", "d", "e", "new"}
	seconds := map[string]float64{"a": 70, "b": 64, "c": 50, "d": 3, "e": 1}
	for parts := 1; parts <= 4; parts++ {
		seen := map[string]int{}
		for _, names := range splitAll(t, packages, seconds, parts) {
			for _, name := range names {
				seen[name]++
			}
		}
		for _, name := range packages {
			if seen[name] != 1 {
				t.Errorf("%d parts: package %q runs %d times, want 1", parts, name, seen[name])
			}
		}
		if len(seen) != len(packages) {
			t.Errorf("%d parts: %d packages run, want %d", parts, len(seen), len(packages))
		}
	}
}

func TestThePartsTakeAboutTheSameTime(t *testing.T) {
	packages := []string{"keys", "application", "acceptance", "sshclient", "httpserver", "sftp", "remotesync", "small1", "small2", "small3"}
	seconds := map[string]float64{"keys": 70, "application": 64, "acceptance": 55, "sshclient": 47, "httpserver": 47, "sftp": 46, "remotesync": 44, "small1": 5, "small2": 4, "small3": 3}
	var totals []float64
	for _, names := range splitAll(t, packages, seconds, 2) {
		total := 0.0
		for _, name := range names {
			total += seconds[name]
		}
		totals = append(totals, total)
	}
	// The heaviest package is 70 of 385 seconds, so an even split is within one package of half.
	if difference := totals[0] - totals[1]; difference > 70 || difference < -70 {
		t.Errorf("part totals = %v, want them within 70 seconds of each other", totals)
	}
}

func TestAPackageWithNoRecordedTimeStillRuns(t *testing.T) {
	names, err := Split([]string{"brand-new"}, map[string]float64{}, 1, 1)
	if err != nil || !slices.Equal(names, []string{"brand-new"}) {
		t.Fatalf("Split = %v, %v; want [brand-new]", names, err)
	}
}

func TestTheSplitDoesNotDependOnTheOrderOfThePackages(t *testing.T) {
	seconds := map[string]float64{"a": 10, "b": 10, "c": 10, "d": 5}
	forward := splitAll(t, []string{"a", "b", "c", "d"}, seconds, 2)
	backward := splitAll(t, []string{"d", "c", "b", "a", "a"}, seconds, 2)
	for part := range forward {
		if !slices.Equal(forward[part], backward[part]) {
			t.Errorf("part %d = %v forward, %v backward", part+1, forward[part], backward[part])
		}
	}
}

func TestAPartOutsideThePartsIsRefused(t *testing.T) {
	for _, bad := range [][2]int{{0, 2}, {3, 2}, {1, 0}} {
		if _, err := Split([]string{"a"}, nil, bad[0], bad[1]); err == nil {
			t.Errorf("Split(part %d of %d) error = nil, want an error", bad[0], bad[1])
		}
	}
}
