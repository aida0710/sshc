package snippets

import (
	"context"
	"sync"
	"time"
)

type jobState struct {
	mutex  sync.Mutex
	view   Job
	cancel context.CancelFunc
	done   chan struct{}
}

// Start verifies the supplied preview evidence and starts one bounded worker
// pool. The caller can cancel through Cancel or by cancelling parent.
func (s *Service) Start(parent context.Context, request ExecuteRequest) (Job, error) {
	preview, planned, err := s.plan(request.PreviewRequest)
	if err != nil {
		return Job{}, err
	}
	if !evidenceMatches(request.Evidence, preview.Evidence) {
		return Job{}, ErrPreviewChanged
	}
	for _, target := range planned {
		if target.run == nil {
			return Job{}, ErrNoRunner
		}
	}
	concurrency := request.Concurrency
	if concurrency == 0 {
		concurrency = DefaultConcurrency
	}
	if concurrency < 1 || concurrency > MaxConcurrency {
		return Job{}, ErrInvalidTarget
	}

	s.jobs.Lock()
	s.pruneJobsLocked()
	if len(s.active) >= MaxRetainedJobs {
		s.jobs.Unlock()
		return Job{}, ErrTooManyJobs
	}
	id, err := s.newJobIDLocked()
	if err != nil {
		s.jobs.Unlock()
		return Job{}, err
	}
	ctx, cancel := context.WithCancel(parent)
	state := &jobState{
		view:   Job{ID: id, Status: JobRunning, StartedAt: s.now().UTC(), Results: make([]TargetResult, len(planned))},
		cancel: cancel, done: make(chan struct{}),
	}
	for index, target := range planned {
		state.view.Results[index] = TargetResult{TargetID: target.targetID, Alias: target.target.Alias, Status: TargetQueued}
	}
	s.active[id] = state
	s.jobs.Unlock()

	go s.execute(ctx, state, planned, concurrency)
	return snapshotJob(state), nil
}

func (s *Service) Job(id string) (Job, error) {
	s.jobs.Lock()
	state := s.active[id]
	s.jobs.Unlock()
	if state == nil {
		return Job{}, ErrUnknownJob
	}
	return snapshotJob(state), nil
}

func (s *Service) Wait(ctx context.Context, id string) (Job, error) {
	s.jobs.Lock()
	state := s.active[id]
	s.jobs.Unlock()
	if state == nil {
		return Job{}, ErrUnknownJob
	}
	select {
	case <-ctx.Done():
		return Job{}, ctx.Err()
	case <-state.done:
		return snapshotJob(state), nil
	}
}

func (s *Service) Cancel(id string) error {
	s.jobs.Lock()
	state := s.active[id]
	s.jobs.Unlock()
	if state == nil {
		return ErrUnknownJob
	}
	state.mutex.Lock()
	finished := state.view.Status != JobRunning
	state.mutex.Unlock()
	if finished {
		return ErrJobFinished
	}
	state.cancel()
	return nil
}

func (s *Service) execute(ctx context.Context, state *jobState, planned []plannedTarget, concurrency int) {
	work := make(chan int)
	var workers sync.WaitGroup
	for range min(concurrency, len(planned)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range work {
				if ctx.Err() != nil {
					continue
				}
				s.setTargetStatus(state, index, TargetRunning)
				output, err := planned[index].run(ctx, planned[index].command)
				state.mutex.Lock()
				result := &state.view.Results[index]
				result.ExitCode = output.ExitCode
				stdout, stdoutCut := publicOutput(output.Stdout, planned[index].secrets)
				stderr, stderrCut := publicOutput(output.Stderr, planned[index].secrets)
				result.Stdout = stdout
				result.Stderr = stderr
				result.Truncated = output.Truncated || stdoutCut || stderrCut
				switch {
				case err != nil && ctx.Err() != nil:
					result.Status = TargetCancelled
					result.Problem = fixedProblem(ctx.Err())
				case err != nil:
					result.Status = TargetFailed
					result.Problem = fixedProblem(err)
				case output.ExitCode != 0:
					result.Status = TargetFailed
				default:
					result.Status = TargetSucceeded
				}
				state.mutex.Unlock()
			}
		}()
	}

dispatch:
	for index := range planned {
		select {
		case work <- index:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(work)
	workers.Wait()

	finished := s.now().UTC()
	state.mutex.Lock()
	state.view.FinishedAt = &finished
	if ctx.Err() != nil {
		state.view.Status = JobCancelled
		for index := range state.view.Results {
			if state.view.Results[index].Status == TargetQueued || state.view.Results[index].Status == TargetRunning {
				state.view.Results[index].Status = TargetCancelled
				state.view.Results[index].Problem = "cancelled"
			}
		}
	} else {
		state.view.Status = JobCompleted
	}
	state.mutex.Unlock()
	state.cancel()
	close(state.done)
}

func (s *Service) setTargetStatus(state *jobState, index int, status TargetStatus) {
	state.mutex.Lock()
	state.view.Results[index].Status = status
	state.mutex.Unlock()
}

func snapshotJob(state *jobState) Job {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	view := state.view
	view.Results = append([]TargetResult(nil), state.view.Results...)
	if state.view.FinishedAt != nil {
		finished := *state.view.FinishedAt
		view.FinishedAt = &finished
	}
	return view
}

func (s *Service) newJobIDLocked() (string, error) {
	for range 8 {
		id, err := s.newID()
		if err != nil {
			return "", err
		}
		if s.active[id] == nil {
			return id, nil
		}
	}
	return "", ErrTooManyJobs
}

func (s *Service) pruneJobsLocked() {
	for len(s.active) >= MaxRetainedJobs {
		var oldestID string
		var oldest time.Time
		for id, state := range s.active {
			state.mutex.Lock()
			finished := state.view.Status != JobRunning
			started := state.view.StartedAt
			state.mutex.Unlock()
			if finished && (oldestID == "" || started.Before(oldest)) {
				oldestID, oldest = id, started
			}
		}
		if oldestID == "" {
			return
		}
		delete(s.active, oldestID)
	}
}

func publicOutput(output []byte, secrets []string) (string, bool) {
	redacted := redact(string(output), secrets)
	if len(redacted) <= MaxResultBytes {
		return redacted, false
	}
	return redacted[:MaxResultBytes], true
}
