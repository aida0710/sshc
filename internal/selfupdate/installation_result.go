package selfupdate

import (
	"time"

	"sshc/internal/releasecheck"
)

func validateInstalledVersion(plan Plan, installedVersion string) error {
	tag, stable := releasecheck.StableTag(installedVersion)
	if !stable || tag != installedVersion {
		return Failure("update_install_failed")
	}
	if installedVersion == plan.Target {
		return nil
	}
	if plan.Installation.Manager != "homebrew" {
		return Failure("update_install_failed")
	}
	// Homebrew follows its formula, which can advance after the confirmation.
	if releasecheck.Newer(plan.Target, installedVersion) {
		return nil
	}
	return Failure("update_homebrew_not_updated")
}

func (service *Service) recordInstallation(id, installedVersion string) (Job, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return service.changeJobLocked(func(job *Job) error {
		if job.ID != id || job.State != JobInstalling {
			return ErrChanged
		}
		job.InstalledVersion = installedVersion
		job.State, job.Problem, job.UpdatedAt = JobRestarting, "", time.Now().UTC()
		return nil
	})
}
