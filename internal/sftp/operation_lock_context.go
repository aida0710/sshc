package sftp

import "context"

type operationLockOutcome struct {
	unlock func()
	err    error
}

// A cancelled caller returns immediately; a pending mutex acquisition releases
// its locks as soon as the current owner finishes. No timer hides that wait.
func (m *TransferManager) LockOperationContext(ctx context.Context, alias string, paths ...string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	outcomes := make(chan operationLockOutcome)
	go func() {
		unlock, err := m.LockOperation(alias, paths...)
		select {
		case outcomes <- operationLockOutcome{unlock: unlock, err: err}:
		case <-ctx.Done():
			if unlock != nil {
				unlock()
			}
		}
	}()
	select {
	case outcome := <-outcomes:
		if err := ctx.Err(); err != nil {
			if outcome.unlock != nil {
				outcome.unlock()
			}
			return nil, err
		}
		return outcome.unlock, outcome.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
