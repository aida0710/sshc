package selfupdate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"sshc/internal/releasecheck"
)

type Service struct {
	dependencies Dependencies
	store        Store
	mutex        sync.Mutex
	stateError   error
	ownedJobID   string
}

func New(dependencies Dependencies) *Service {
	if dependencies.Context == nil {
		dependencies.Context = context.Background()
	}
	return &Service{dependencies: dependencies, store: Store{Path: dependencies.StatePath}}
}

// Inspect reports the installation boundary even when there is no newer release.
func (service *Service) Inspect(ctx context.Context) (Installation, error) {
	if _, ok := releasecheck.StableTag(service.dependencies.Current); !ok {
		return Installation{}, Failure("update_development_build")
	}
	if service.dependencies.Inspect == nil || service.dependencies.Install == nil || service.dependencies.Restart == nil {
		return Installation{}, ErrUnavailable
	}
	return service.dependencies.Inspect(ctx)
}

func (service *Service) Prepare(ctx context.Context, target string) (Plan, error) {
	tag, ok := releasecheck.StableTag(target)
	if !ok || tag != target {
		return Plan{}, ErrChanged
	}
	job, err := service.Status()
	if err != nil {
		return Plan{}, err
	}
	if job.Active() {
		return Plan{}, ErrBusy
	}
	if job.State == JobRestartRequired {
		return Plan{}, Failure("update_restart_failed")
	}
	installation, err := service.Inspect(ctx)
	if err != nil {
		return Plan{}, err
	}
	if service.dependencies.Latest == nil {
		return Plan{}, ErrUnavailable
	}
	latest, err := service.dependencies.Latest(ctx)
	if err != nil {
		return Plan{}, Failure("update_check_failed")
	}
	if latest.Version != target || !releasecheck.Newer(service.dependencies.Current, target) {
		return Plan{}, ErrChanged
	}
	return Plan{Current: service.dependencies.Current, Target: target, Installation: installation}, nil
}

// Start reserves a job. Work starts only after the HTTP adapter reports that the
// accepted response has been flushed; the domain owns that ordering.
func (service *Service) Start(plan Plan) (Job, error) {
	if plan.Current != service.dependencies.Current || !releasecheck.Newer(plan.Current, plan.Target) {
		return Job{}, ErrChanged
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	job, err := service.store.Change(func(job *Job) error {
		if job.Active() {
			return ErrBusy
		}
		if job.State == JobRestartRequired {
			return Failure("update_restart_failed")
		}
		nonce := make([]byte, jobNonceBytes)
		if _, err := rand.Read(nonce); err != nil {
			return ErrState
		}
		*job = Job{ID: hex.EncodeToString(nonce), Target: plan.Target, State: JobAccepted, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), OwnerPID: service.dependencies.PID, Plan: plan}
		return nil
	})
	if err == nil {
		service.ownedJobID = job.ID
	}
	return job, err
}

func (service *Service) ResponseSent(id string, responseError error) {
	if responseError != nil {
		service.finish(id, JobFailed, "update_response_failed")
		return
	}
	go service.install(id)
}

func (service *Service) install(id string) {
	job, err := service.transition(id, JobAccepted, JobInstalling)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(service.dependencies.Context, InstallationTimeout)
	defer cancel()
	installation, err := service.Inspect(ctx)
	if err == nil && installation != job.Plan.Installation {
		err = ErrChanged
	}
	if err == nil {
		err = service.dependencies.Install(ctx, job.Plan)
	}
	if err != nil {
		code := failureCode(err, "update_install_failed")
		if errors.Is(ctx.Err(), context.Canceled) {
			code = "update_interrupted"
		}
		service.finish(id, JobFailed, code)
		return
	}
	// Never restart without a durable installation result.
	job, err = service.transition(id, JobInstalling, JobRestarting)
	if err != nil {
		return
	}
	if err := service.dependencies.Restart(ctx, job); err != nil {
		service.finish(id, JobRestartRequired, failureCode(err, "update_restart_failed"))
	}
}

// Status reconciles with the version of the new engine, including a helper that
// was stopped by systemd/launchd after it submitted the restart request.
func (service *Service) Status() (Job, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.stateError != nil {
		return Job{}, service.stateError
	}
	job, err := service.store.Read()
	if err != nil || (!job.Active() && job.State != JobRestartRequired) {
		return job, err
	}
	// A restarted process may reuse the old PID; only jobs reserved by this
	// service instance can still have an installer running here.
	if job.ID == service.ownedJobID && job.Target != service.dependencies.Current && (job.State != JobRestarting || time.Since(job.UpdatedAt) < RestartTimeout) {
		return job, nil
	}
	return service.store.Change(func(stored *Job) error {
		if !stored.Active() && stored.State != JobRestartRequired {
			return nil
		}
		if stored.Target == service.dependencies.Current {
			stored.State, stored.Problem = JobSucceeded, ""
		} else if stored.State == JobRestarting || stored.State == JobRestartRequired {
			stored.State, stored.Problem = JobRestartRequired, "update_restart_failed"
		} else {
			stored.State, stored.Problem = JobFailed, "update_interrupted"
		}
		stored.UpdatedAt = time.Now().UTC()
		return nil
	})
}

func (service *Service) transition(id, from, to string) (Job, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	job, err := service.store.Change(func(job *Job) error {
		if job.ID != id || job.State != from {
			return ErrChanged
		}
		job.State, job.Problem, job.UpdatedAt = to, "", time.Now().UTC()
		return nil
	})
	if errors.Is(err, ErrState) {
		service.stateError = err
	}
	return job, err
}

func (service *Service) finish(id, state, problem string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	_, err := service.store.Change(func(job *Job) error {
		if job.ID != id || !job.Active() {
			return ErrChanged
		}
		job.State, job.Problem, job.UpdatedAt = state, problem, time.Now().UTC()
		return nil
	})
	if errors.Is(err, ErrState) {
		service.stateError = err
	}
}

func failureCode(err error, fallback string) string {
	var failure Failure
	if errors.As(err, &failure) {
		return string(failure)
	}
	return fallback
}
