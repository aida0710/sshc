package application

// AbsoluteGroupsPath は、グループの共通設定を書くファイル（metadata の GroupsPath()）の
// 絶対パスを返す。config.Resolver.GroupsFile に渡すと、internal/effective はそのファイルの
// Host 行を alias の宣言として数えない。
//
// metadata を読めなければ空を返す。そのときは重複の印が残るだけで、値と出所は変わらない。
// 読めないことは、metadata を使うほかの操作が知らせる。
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
