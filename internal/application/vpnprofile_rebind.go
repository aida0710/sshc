package application

// VPN プロファイルの改名と削除に合わせて、保存済みのパスワード・TOTP・起動スニペットの
// 割り当てが持つ結び付けの値（sshclient.Target.AuthenticationBinding）を書き換える。
// 結び付けの値は、接続に付けたプロファイルの名前を含むからである。

// vpnProfileTransition は、プロファイルひとつの改名か削除で、metadata の hosts が
// どう変わるかである。
type vpnProfileTransition struct {
	// profile は、改名か削除をするプロファイルの、変える前の名前である。
	profile string
	// before と after は、変える前と後の hosts である。改名なら after では新しい名前を、
	// 削除なら空を指す。
	before, after []HostMetadata
}

// assignmentRebind は、transition のプロファイルを通る接続の、いま有効な割り当てを
// 書き換える関数を返す（約束は VPNProfileChange.AssignmentRebind のとおり）。
//
// after でもプロファイルを通る接続（改名）は、after での結び付けの値へ移す。経路は
// 変わらないからである。after でプロファイルを外された接続（削除）は、空の値を返して
// 割り当てを停止中にする。同じ名前で作り直したプロファイルは別の VPN でありうるので、
// それを付け直しても前の割り当てが有効に戻らないようにする。
//
// 結び付けの値がいまの設定と合わない割り当ては、すでに停止中なので触れない。設定は
// 計画のときに読んだものを使う。書くまでに設定が変わっても、書き換えるのはプロファイルの
// 名前だけが違う値へなので、別の経路の割り当てにはならない。
func (s *Service) assignmentRebind(transition vpnProfileTransition) (func(alias, binding string) (string, bool), error) {
	graph, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return func(alias, binding string) (string, bool) {
		identity := s.connectionIdentity(graph, alias)
		if vpnProfileOf(transition.before, identity) != transition.profile {
			return "", false
		}
		current, err := s.passwordBindingForGraph(graph, transition.before, alias)
		if err != nil || current != binding {
			return "", false
		}
		if vpnProfileOf(transition.after, identity) == "" {
			return "", true
		}
		rebound, err := s.passwordBindingForGraph(graph, transition.after, alias)
		return rebound, err == nil
	}, nil
}
