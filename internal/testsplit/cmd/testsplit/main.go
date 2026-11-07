// Command testsplit prints, one per line, the Go packages whose tests one CI job runs:
//
//	go run ./internal/testsplit/cmd/testsplit -part 1 -of 2
//
// It balances the parts by the times in scripts/ci/go-test-seconds.json. To record the times
// again, save the logs of the Go test jobs of one CI run on main and give them to -record:
//
//	go run ./internal/testsplit/cmd/testsplit -record linux.log macos.log windows.log
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"

	"sshc/internal/testsplit"
)

// secondsFile is where the recorded times are, from the root of the repository, where CI runs.
const secondsFile = "scripts/ci/go-test-seconds.json"

func main() {
	part := flag.Int("part", 1, "which part to print, counted from 1")
	parts := flag.Int("of", 1, "how many parts the packages are split into")
	record := flag.Bool("record", false, "record the package times from the go test logs given as arguments")
	flag.Parse()
	var err error
	if *record {
		err = recordTimes(flag.Args())
	} else {
		err = printPart(*part, *parts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testsplit:", err)
		os.Exit(1)
	}
}

func printPart(part, parts int) error {
	seconds := map[string]float64{}
	data, err := os.ReadFile(secondsFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &seconds); err != nil {
			return fmt.Errorf("%s: %w", secondsFile, err)
		}
	}
	listed, err := exec.Command("go", "list", "./...").Output()
	if err != nil {
		return fmt.Errorf("go list: %w", err)
	}
	names, err := testsplit.Split(strings.Fields(string(listed)), seconds, part, parts)
	if err != nil {
		return err
	}
	fmt.Println(strings.Join(names, "\n"))
	return nil
}

func recordTimes(logs []string) error {
	if len(logs) == 0 {
		return errors.New("-record needs the go test logs as arguments")
	}
	seconds := map[string]float64{}
	for _, path := range logs {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		err = testsplit.RecordSeconds(file, seconds)
		file.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	for name, value := range seconds {
		seconds[name] = math.Round(value*10) / 10
	}
	data, err := json.MarshalIndent(seconds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(secondsFile, append(data, '\n'), 0o644)
}
