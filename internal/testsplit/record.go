package testsplit

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
)

// passedPackage matches the line go test prints for a package whose tests passed, such as
// "ok  	sshc/internal/keys	70.372s", also with a CI log's timestamp in front of it.
var passedPackage = regexp.MustCompile(`(?:^|\s)ok\s+(\S+)\s+([0-9]+(?:\.[0-9]+)?)s(?:\s|$)`)

// RecordSeconds adds to seconds the time of every package that passed in a go test output, keeping
// the larger of two times for the same package: the parts are balanced for the slowest OS.
func RecordSeconds(output io.Reader, seconds map[string]float64) error {
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		match := passedPackage.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		if value > seconds[match[1]] {
			seconds[match[1]] = value
		}
	}
	return scanner.Err()
}
