package knownhosts

// ConnectionName は、保存した接続先の alias と、ssh がその鍵を known_hosts で探す名前
// （HostField の形か HostKeyAlias）である。
type ConnectionName struct {
	Alias string
	Field string
}

// AliasesAmong は、names のうち、このエントリが鍵を持つ接続先の alias を names の順に返す。
// ハッシュ化されたエントリは読み戻せないが、接続先の名前を当てれば確かめられるので、
// どの接続先のものかが分かる。
func (e *Entry) AliasesAmong(names []ConnectionName) []string {
	var aliases []string
	for _, name := range names {
		if e.MatchesHost(name.Field) {
			aliases = append(aliases, name.Alias)
		}
	}
	return aliases
}
