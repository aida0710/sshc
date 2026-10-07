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
	dependencies  Dependencies
	store         Store
	mutex         sync.Mutex
	stateError    error
	ownedJobID    string
	workerContext context.Context
	cancelWorkers context.CancelFunc
	stopping      bool
	workers       sync.WaitGroup
	waitOnce      sync.Once
	waitError     error
}

func New(dependencies Dependencies) *Service {
	if dependencies.Context == nil {
		dependencies.Context = context.Background()
	}
	workerContext, cancelWorkers := context.WithCancel(dependencies.Context)
	return &Service{
		dependencies: dependencies, store: Store{Path: dependencies.StatePath},
		workerContext: workerContext, cancelWorkers: cancelWorkers,
	}
}

// Inspect reports the installation boundary even when there is no newer release.
func (service *Service) Inspect(ctx context.Context) (Installation, error) {
	if service.workerContext.Err() != nil {
		return Installation{}, ErrUnavailable
	}
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
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.stopping || service.workerContext.Err() != nil {
		return Job{}, ErrUnavailable
	}
	if plan.Current != service.dependencies.Current || !releasecheck.Newer(plan.Current, plan.Target) {
		return Job{}, ErrChanged
	}
	job, err := service.changeJobLocked(func(job *Job) error {
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
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.stopping || service.workerContext.Err() != nil || id != service.ownedJobID {
		return
	}
	job, err := service.transitionLocked(id, JobAccepted, JobInstalling)
	if err != nil {
		return
	}
	if responseError != nil {
		_ = service.finishLocked(id, JobFailed, "update_response_failed")
		return
	}
	// Register under the stopping gate so Wait can never miss a new worker.
	service.workers.Add(1)
	go func() {
		defer service.workers.Done()
		service.install(job)
	}()
}

func (service *Service) install(job Job) {
	ctx, cancel := context.WithTimeout(service.workerContext, InstallationTimeout)
	defer cancel()
	if err := service.installPlan(ctx, job.Plan); err != nil {
		code := failureCode(err, "update_install_failed")
		if errors.Is(ctx.Err(), context.Canceled) {
			code = "update_interrupted"
		}
		service.finish(job.ID, JobFailed, code)
		return
	}
	// Never restart without a durable installation result.
	job, err := service.transition(job.ID, JobInstalling, JobRestarting)
	if err != nil {
		return
	}
	if err = ctx.Err(); err == nil {
		err = service.dependencies.Restart(ctx, job)
	}
	if err != nil {
		service.finish(job.ID, JobRestartRequired, failureCode(err, "update_restart_failed"))
	}
}

func (service *Service) installPlan(ctx context.Context, plan Plan) error {
	installation, err := service.Inspect(ctx)
	if err != nil {
		return err
	}
	if installation != plan.Installation {
		return ErrChanged
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return service.dependencies.Install(ctx, plan)
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
	return service.changeJobLocked(func(stored *Job) error {
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
	return service.transitionLocked(id, from, to)
}

func (service *Service) transitionLocked(id, from, to string) (Job, error) {
	return service.changeJobLocked(func(job *Job) error {
		if job.ID != id || job.State != from {
			return ErrChanged
		}
		job.State, job.Problem, job.UpdatedAt = to, "", time.Now().UTC()
		return nil
	})
}

func (service *Service) finish(id, state, problem string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	_ = service.finishLocked(id, state, problem)
}

func (service *Service) finishLocked(id, state, problem string) error {
	_, err := service.changeJobLocked(func(job *Job) error {
		if job.ID != id || !job.Active() {
			return ErrChanged
		}
		job.State, job.Problem, job.UpdatedAt = state, problem, time.Now().UTC()
		return nil
	})
	return err
}

// changeJobLocked retains persistence failures while service.mutex is held.
func (service *Service) changeJobLocked(change func(*Job) error) (Job, error) {
	job, err := service.store.Change(change)
	if errors.Is(err, ErrState) {
		service.stateError = err
	}
	return job, err
}

func failureCode(err error, fallback string) string {
	var failure Failure
	if errors.As(err, &failure) {
		return string(failure)
	}
	return fallback
}
