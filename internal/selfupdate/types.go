// Package selfupdate owns the update job and its restart recovery. Installers
// and process startup are supplied by the desktop composition root.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"sshc/internal/releasecheck"
)

const (
	// StateFileName stores the update result on this device only.
	StateFileName = "web-update.json"
	// InstallationTimeout bounds downloads/builds without tying them to a browser request.
	InstallationTimeout = 15 * time.Minute
	// RestartTimeout allows engine shutdown and service readiness before asking for manual recovery.
	RestartTimeout = 2 * time.Minute
)

// Job phases are durable; only a new engine reporting Target confirms success.
const (
	JobAccepted        = "accepted"
	JobInstalling      = "installing"
	JobRestarting      = "restarting"
	JobSucceeded       = "succeeded"
	JobFailed          = "failed"
	JobRestartRequired = "restart_required"
	// Job IDs use 128 random bits and carry no authentication authority.
	jobNonceBytes = 16
	jobIDLength   = 2 * jobNonceBytes
	// Sibling locks remain stable across the state's atomic rename.
	StateLockSuffix   = ".lock"
	RestartLockSuffix = ".restart.lock"
)

type Installation struct {
	Manager    string `json:"manager"`
	Executable string `json:"executable"`
	Identity   string `json:"identity"`
}

type Plan struct {
	Current      string       `json:"current"`
	Target       string       `json:"target"`
	Installation Installation `json:"installation"`
}

func (plan Plan) Evidence() string {
	body, _ := json.Marshal(plan)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

type Job struct {
	ID        string    `json:"id"`
	Target    string    `json:"target"`
	State     string    `json:"state"`
	Problem   string    `json:"problem"`
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	OwnerPID  int       `json:"ownerPid"`
	Plan      Plan      `json:"plan"`
}

func (job Job) Active() bool {
	return job.State == JobAccepted || job.State == JobInstalling || job.State == JobRestarting
}

type Failure string

func (failure Failure) Error() string { return string(failure) }

const (
	ErrBusy        Failure = "update_in_progress"
	ErrUnavailable Failure = "update_unavailable"
	ErrChanged     Failure = "update_plan_changed"
	ErrState       Failure = "update_state_failed"
	ErrPermission  Failure = "update_permission_denied"
)

type Dependencies struct {
	Current   string
	PID       int
	StatePath string
	Context   context.Context
	Latest    func(context.Context) (releasecheck.Release, error)
	Inspect   func(context.Context) (Installation, error)
	Install   func(context.Context, Plan) error
	Restart   func(context.Context, Job) error
}
