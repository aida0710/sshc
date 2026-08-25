package application

import (
	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/sshclient"
)

// PasswordBinding returns the resolved authentication destination digest for an
// alias. It includes every ProxyJump hop and the settings that decide which peer
// receives an account password.
func (s *Service) PasswordBinding(alias string) (string, error) {
	graph, err := s.resolve()
	if err != nil {
		return "", err
	}
	return s.passwordBindingForGraph(graph, alias)
}

func (s *Service) passwordBindingForGraph(graph *config.Graph, alias string) (string, error) {
	resolve := func(candidate string) (effective.Values, error) {
		resolution := effective.Resolve(graph, candidate, s.localFacts())
		if len(resolution.Refusals) == 0 {
			return resolution.Values, nil
		}
		failure := &ErrUnresolvable{Alias: candidate}
		for _, refusal := range resolution.Refusals {
			failure.Codes = append(failure.Codes, refusal.Code)
			failure.Details = append(failure.Details, refusal.Detail)
		}
		return effective.Values{}, failure
	}
	target, err := sshclient.NewTarget(alias, resolve, s.workspace.Home())
	if err != nil {
		return "", err
	}
	return target.AuthenticationBinding(), nil
}
