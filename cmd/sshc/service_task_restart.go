package main

import (
	"context"
	"fmt"
)

// The context belongs to one operation, including all definition rechecks.
type taskRestartDefinition struct {
	manager *taskServiceManager
	ctx     context.Context
}

func (manager *taskServiceManager) taskDefinition(ctx context.Context) taskRestartDefinition {
	return taskRestartDefinition{manager: manager, ctx: ctx}
}
func (manager *taskServiceManager) restartDefinition(ctx context.Context) serviceRestartDefinition {
	return manager.taskDefinition(ctx)
}
func (definition taskRestartDefinition) matches(executable string) (bool, error) {
	snapshot, err := definition.manager.currentTask(definition.ctx)
	return err == nil && snapshot.exists && snapshot.definition.managedBy(definition.manager.sid) && snapshot.definition.matches(executable, definition.manager.sid), err
}
func (definition taskRestartDefinition) ensureStillMatches(executable, activity string) error {
	matches, err := definition.matches(executable)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("sshc task changed while the service was %s", activity)
	}
	return nil
}
