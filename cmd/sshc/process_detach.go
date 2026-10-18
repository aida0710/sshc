package main

import "os/exec"

type processExit struct {
	code int
	err  error
}

type detachedProcess struct {
	pid     int
	exited  <-chan processExit
	release func()
}

// startDetachedProcess retries only the Windows job-policy refusal. A fresh Cmd
// also reopens any streams that exec closed after the failed Start.
func startDetachedProcess(newCommand func() *exec.Cmd) (*detachedProcess, error) {
	command := newCommand()
	configureDetachedProcess(command)
	err := command.Start()
	if shouldRetryWithoutBreakaway(err) {
		command = newCommand()
		configureDetachedProcess(command)
		clearProcessBreakaway(command)
		err = command.Start()
	}
	if err != nil {
		return nil, err
	}
	return watchDetachedProcess(command)
}
