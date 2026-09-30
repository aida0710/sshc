package application

import (
	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/sshclient"
)

// PasswordBinding returns the resolved authentication destination digest for an
// alias. It includes every ProxyJump hop, the VPN profile attached to the
// connection and the settings that decide which peer receives an account password.
func (s *Service) PasswordBinding(alias string) (string, error) {
	graph, err := s.resolve()
	if err != nil {
		return "", err
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return "", err
	}
	return s.passwordBindingForGraph(graph, stored.Hosts, alias)
}

// passwordBindingForGraph は、読み終えた設定グラフと metadata の hosts から、接続が
// 照合するのと同じ結び付けの値を返す。
func (s *Service) passwordBindingForGraph(graph *config.Graph, hosts []HostMetadata, alias string) (string, error) {
	target, err := s.targetForGraph(graph, alias)
	if err != nil {
		return "", err
	}
	// 接続（internal/app の sshParts.target）と同じく、VPN は行き先の alias にだけ付ける。
	// ProxyJump のホップは VPN を通らない。片方だけが VPN を含むと、VPN を付けた接続では
	// 保存済みの値が一度も照合に通らない。
	target.VPN = vpnProfileOf(hosts, s.connectionIdentity(graph, alias))
	return target.AuthenticationBinding(), nil
}

// targetForGraph は、読み終えた設定グラフから、接続が使うのと同じ Target を組み立てる。
// VPN は metadata にあるので入れない。結び付けの値は passwordBindingForGraph で作る。
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
