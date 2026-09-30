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
	target, err := s.targetForGraph(graph, alias)
	if err != nil {
		return "", err
	}
	return target.AuthenticationBinding(), nil
}

// targetForGraph は、読み終えた設定グラフから、接続が使うのと同じ Target を組み立てる。
func (s *Service) targetForGraph(graph *config.Graph, alias string) (sshclient.Target, error) {
	facts := s.localFacts()
	resolve := func(candidate string) (effective.Values, error) {
		resolution := effective.Resolve(graph, candidate, facts)
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
	return sshclient.NewTarget(alias, resolve, facts)
}
