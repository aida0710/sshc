package application

import (
	"sshc/internal/config"
	"sshc/internal/effective"
)

// 接続に付ける sshc の設定（VPN、文字コード、OS）を、どの Host ブロックが持つかを決める。

// connectionIdentity は、alias で接続したときに sshc の設定を持つ Host ブロックを返す。
//
// 接続先を決める ResolveConnection と同じく大文字と小文字を区別し、alias をそのまま
// 宣言している最初のブロックを取る。否定のパターンで alias を除くブロックは取らない。
// 別名（`Host web web-alt` の web-alt）で接続しても、そのブロックを取る。外部のファイルの
// ブロックは sshc の設定を持てないので、ゼロ値を返す。後ろにある同じ名前のブロックの
// 設定は借りない。
func (s *Service) connectionIdentity(graph *config.Graph, alias string) HostIdentity {
	var identity HostIdentity
	WalkDirectives(graph, func(visit Visit) bool {
		block := visit.Block
		if block.Kind != config.BlockHost || block.Header != visit.Index {
			return true
		}
		if !effective.DeclaresExactly(block.Patterns, alias) || excludedByNegation(block.Patterns, alias) {
			return true
		}
		if file := NewFileRef(s.workspace.Root(), visit.Path); !file.External {
			identity = HostIdentity{Path: file.Path, Alias: PrimaryAlias(block.Patterns)}
		}
		return false
	})
	return identity
}

// excludedByNegation は、否定のパターン（`!web`）が alias を除くかを返す。
func excludedByNegation(patterns []config.Pattern, alias string) bool {
	for _, pattern := range patterns {
		if pattern.Negated && effective.MatchPattern(pattern.Value, alias) {
			return true
		}
	}
	return false
}

// connectingAlias は、identity のブロックへ接続する alias を返す。ブロックが宣言している
// alias のうち、connectionIdentity でそのブロックに決まる最初のものを選ぶ。ブロックが無い
// か、宣言している alias をすべて前のブロックが先に宣言しているなら、false を返す。
func (s *Service) connectingAlias(graph *config.Graph, identity HostIdentity) (string, bool) {
	var declared []string
	WalkDirectives(graph, func(visit Visit) bool {
		block := visit.Block
		if block.Kind != config.BlockHost || block.Header != visit.Index {
			return true
		}
		file := NewFileRef(s.workspace.Root(), visit.Path)
		if file.External || file.Path != identity.Path || PrimaryAlias(block.Patterns) != identity.Alias {
			return true
		}
		for _, pattern := range block.Patterns {
			if !pattern.Negated && !pattern.Wildcard {
				declared = append(declared, pattern.Value)
			}
		}
		return false
	})
	for _, alias := range declared {
		if s.connectionIdentity(graph, alias) == identity {
			return alias, true
		}
	}
	return "", false
}
