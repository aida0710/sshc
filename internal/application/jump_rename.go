package application

import (
	"path/filepath"
	"strings"

	"sshc/internal/config"
	"sshc/internal/storage"
)

// 接続先の名前を変えるときは、その名前を踏み台（ProxyJump）に書いているほかの接続先も、同じ
// トランザクションで新しい名前に書き換える。書き換えないと、OpenSSH は古い名前を接続先の名前として
// 引き（定義の無い名前はホスト名として扱われる）、踏み台を通らないまま「名前を引けない」で失敗する。

// renameJumpHops は、file の ProxyJump のうち、ホップの接続先が from であるものを to に書き換え、
// 書き換えたかを返す。
func renameJumpHops(file *config.File, from, to string) (bool, error) {
	changed := false
	for index, line := range file.Lines {
		if !config.EqualKeyword(line.Keyword, "ProxyJump") {
			continue
		}
		values := line.Values()
		if len(values) == 0 {
			continue
		}
		hops, renamed := renamedHops(values[0], from, to)
		if !renamed {
			continue
		}
		values[0] = hops
		rebuilt, err := rebuildLine(line, line.Keyword, values)
		if err != nil {
			return false, err
		}
		file.Lines[index] = rebuilt
		changed = true
	}
	return changed, nil
}

// renamedHops は、ProxyJump の値（[user@]host[:port] をカンマでつないだもの。ssh:// の URI も
// 書ける）のうち、host が from のホップを to に書き換える。OpenSSH は接続先の名前を小文字にして
// Host と照らすので、大小文字は区別しない。[ ] で囲んだ IPv6 のアドレスは接続先の名前ではない。
func renamedHops(value, from, to string) (string, bool) {
	hops := strings.Split(value, ",")
	renamed := false
	for index, hop := range hops {
		rest := hop
		scheme := ""
		if strings.HasPrefix(strings.ToLower(rest), "ssh://") {
			scheme, rest = rest[:len("ssh://")], rest[len("ssh://"):]
		}
		user := ""
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			user, rest = rest[:at+1], rest[at+1:]
		}
		if strings.HasPrefix(rest, "[") {
			continue
		}
		host, port := rest, ""
		if colon := strings.LastIndex(rest, ":"); colon >= 0 {
			host, port = rest[:colon], rest[colon:]
		}
		if !strings.EqualFold(host, from) {
			continue
		}
		hops[index] = scheme + user + to + port
		renamed = true
	}
	return strings.Join(hops, ","), renamed
}

// addJumpRenames は、名前を変えるファイル（renamed）のほかの、書ける設定ファイルの ProxyJump を
// 書き換え、prepared に加える。読めない・書けないファイルは飛ばす（そこに残った古い名前は、
// 踏み台の欄の注意で分かる）。
func (s *Service) addJumpRenames(graph *config.Graph, prepared *planned, renamed, from, to string) error {
	for _, path := range graph.Order {
		node := graph.Nodes[path]
		if node == nil || node.Missing || !node.Editable || filepath.Clean(path) == filepath.Clean(renamed) {
			continue
		}
		if _, err := s.workspace.ResolveForWrite(path); err != nil {
			continue
		}
		disk, exists, err := s.readConfigurationFile(path)
		if err != nil || !exists {
			continue
		}
		file := config.Parse(disk)
		changed, err := renameJumpHops(file, from, to)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := s.addChange(prepared, storage.Change{Path: path, Contents: file.Render(), Precondition: preconditionFor(disk, exists)}); err != nil {
			return err
		}
	}
	return nil
}
