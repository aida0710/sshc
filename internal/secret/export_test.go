package secret

import "sshc/internal/storage"

// テストからだけ使う入口。

// TransactionsForTest は、この Service が vault を書くときの storage.Manager を返す。
// テストは secrettest の前準備に、本番の配線と同じ Manager を渡す。
func (s *Service) TransactionsForTest() *storage.Manager {
	return s.transactions
}
