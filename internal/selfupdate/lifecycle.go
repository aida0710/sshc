package selfupdate

// BeginStopping closes the reservation/response gate and cancels workers. It
// does not wait, so an HTTP stop response or a restart launch can still finish.
func (service *Service) BeginStopping() {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.stopping = true
	service.cancelWorkers()
}

// Wait must follow BeginStopping. Install includes the installer's process Wait
// and group cleanup; Restart returns after launching its independent helper.
// Waiting for that helper to exit here would deadlock its engine stop request.
func (service *Service) Wait() error {
	service.waitOnce.Do(func() {
		service.workers.Wait()
		service.mutex.Lock()
		defer service.mutex.Unlock()
		if service.stateError != nil {
			service.waitError = service.stateError
			return
		}
		job, err := service.store.Read()
		if err != nil {
			service.waitError = err
			return
		}
		// A response may still be flushing when stopping begins. No worker owns
		// that accepted job, but it must have a durable interruption result too.
		if job.ID == service.ownedJobID && (job.State == JobAccepted || job.State == JobInstalling) {
			service.waitError = service.finishLocked(job.ID, JobFailed, "update_interrupted")
		}
	})
	return service.waitError
}

func (service *Service) Stop() error {
	service.BeginStopping()
	return service.Wait()
}
