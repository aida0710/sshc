package application

import "sshc/internal/knownhosts"

// KnownHostNames は、保存した接続先ごとに、ssh がその鍵を known_hosts で探す名前を返す。
// 接続と同じ Target から作るので、HostKeyAlias も Port も接続と同じに効く。解決できない
// 接続先（Match exec を含む設定など）と、先に宣言された alias の重複は含めない。
func (s *Service) KnownHostNames() ([]knownhosts.ConnectionName, error) {
	graph, err := s.resolve()
	if err != nil {
		return nil, err
	}
	hosts, _ := ProjectHosts(graph, s.workspace.Root())
	names := make([]knownhosts.ConnectionName, 0, len(hosts))
	for _, host := range hosts {
		alias := host.Identity.Alias
		if alias == "" || host.Duplicate {
			continue
		}
		target, err := s.targetForGraph(graph, alias)
		if err != nil {
			continue
		}
		names = append(names, knownhosts.ConnectionName{Alias: alias, Field: target.KnownHostsField()})
	}
	return names, nil
}
