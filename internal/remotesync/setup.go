package remotesync

import (
	"context"
	"errors"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
)

type SetupTargetState string

const (
	SetupTargetEmpty      SetupTargetState = "empty"
	SetupTargetExisting   SetupTargetState = "existing"
	SetupTargetIncomplete SetupTargetState = "incomplete"
)

var (
	ErrSetupTargetChanged    = errors.New("the synchronization target changed after connection check")
	ErrSetupTargetIncomplete = errors.New("the synchronization target has history but no live snapshot")
)

type SetupInspection struct {
	State          SetupTargetState
	ETag           string
	HistoryPresent bool
}

// InspectSetupTarget checks the exact live key and its history prefix without
// changing local state or persisting credentials.
func InspectSetupTarget(ctx context.Context, client *objectstore.Client, config Config) (SetupInspection, error) {
	liveETag, err := client.Head(ctx, ObjectKeyFor(config))
	live := err == nil
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return SetupInspection{}, err
	}
	history, _, err := client.ListNewest(ctx, joinKey(config.Path, SnapshotPrefix), 1)
	if err != nil {
		return SetupInspection{}, err
	}
	inspection := SetupInspection{ETag: liveETag, HistoryPresent: len(history) > 0}
	switch {
	case live:
		inspection.State = SetupTargetExisting
	case inspection.HistoryPresent:
		inspection.State = SetupTargetIncomplete
	default:
		inspection.State = SetupTargetEmpty
	}
	return inspection, nil
}

func sameSetupInspection(left, right SetupInspection) bool {
	return left.State == right.State && left.ETag == right.ETag && left.HistoryPresent == right.HistoryPresent
}

func verifySetupKey(ctx context.Context, client *objectstore.Client, config Config, expectedETag, key string) error {
	object, err := client.Get(ctx, ObjectKeyFor(config))
	if err != nil {
		return err
	}
	if object.ETag != expectedETag {
		return ErrSetupTargetChanged
	}
	archive, _, err := envelope.OpenWithin(object.Body, key, envelope.AcceptedFromRemote)
	if err != nil {
		return err
	}
	_, _, err = Read(archive)
	return err
}

// CompleteSetup rechecks the target, validates an existing snapshot key, then
// persists settings and publishes the binding under one operation boundary.
func (s *Service) CompleteSetup(ctx context.Context, config Config, credentials objectstore.Credentials,
	client *objectstore.Client, expected SetupInspection, key string, persist func() error) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	config = normalizeConfig(config)
	if persist == nil {
		return errors.New("remote sync settings persistence is not configured")
	}
	if err := s.validateRecoveryTarget(config); err != nil {
		return err
	}
	actual, err := InspectSetupTarget(ctx, client, config)
	if err != nil {
		return err
	}
	if !sameSetupInspection(expected, actual) {
		return ErrSetupTargetChanged
	}
	if actual.State == SetupTargetIncomplete {
		return ErrSetupTargetIncomplete
	}
	if err := ValidateKey(key); err != nil {
		return err
	}
	if actual.State == SetupTargetExisting {
		if err := verifySetupKey(ctx, client, config, actual.ETag, key); err != nil {
			return err
		}
	}
	if err := persist(); err != nil {
		return err
	}
	s.configure(config, credentials, client)
	return nil
}
