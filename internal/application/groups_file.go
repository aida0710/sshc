package application

// AbsoluteGroupsPath は、グループの共通設定を書くファイル（metadata の GroupsPath()）の
// 絶対パスを返す。config.Resolver.GroupsFile に渡すと、internal/effective はそのファイルの
// Host 行を alias の宣言として数えず、ProjectHosts は接続先として返さない。
//
// metadata を読めなければ空を返す。そのときはその Host 行が接続先と alias の重複として
// 見えるだけで、値と出所は変わらない。読めないことは、metadata を使うほかの操作が知らせる。
func (s *Service) AbsoluteGroupsPath() string {
	stored, _, err := s.metadata.Load()
	if err != nil {
		return ""
	}
	path, err := AbsolutePath(s.workspace.Root(), stored.GroupsPath())
	if err != nil {
		return ""
	}
	return path
}
