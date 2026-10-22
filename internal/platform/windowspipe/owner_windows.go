//go:build windows

package windowspipe

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// ErrUntrustedPipeOwner は、pipe を作ったのが、鍵を渡してよい相手ではないことを表す。
var ErrUntrustedPipeOwner = errors.New("the named pipe was created by an untrusted owner")

// verifyPipeOwner は、開いた pipe の持ち主が、鍵を渡してよい相手かを確かめる。
//
// named pipe の名前は早い者勝ちである。ssh-agent サービスが止まっている
// Windows（既定で無効）では、同じマシンにログオンした別のユーザーが先に同じ名前の
// pipe を作れる。そこへ繋げば、Keys 画面で登録した秘密鍵がその相手へ届く。
//
// 相手のプロセスのトークンではなく、pipe の持ち主を見る。標準ユーザーは
// LocalSystem で動くサービスのトークンを開けないので、トークンを条件にすると本物の
// agent まで断ってしまう。持ち主は pipe を作った者が決め、自分のユーザーか、
// 持ち主になれる自分のグループにしかできない。開いた handle で読めるので、追加の
// 権限も要らない。
func verifyPipeOwner(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read the named pipe owner: %w", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read the named pipe owner: %w", err)
	}
	if owner == nil {
		return ErrUntrustedPipeOwner
	}
	currentUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("read the current user: %w", err)
	}
	if !isTrustedPipeOwner(owner, currentUser.User.Sid) {
		return fmt.Errorf("%w: %s", ErrUntrustedPipeOwner, owner)
	}
	return nil
}

// isTrustedPipeOwner は、鍵を渡してよい pipe の持ち主かを答える。
//
// OpenSSH の ssh-agent サービスは LocalSystem で動く。LocalSystem が作った
// pipe の持ち主は、作り方によって LocalSystem か Administrators になる。昇格した
// 管理者が作ったものも Administrators になる。Administrators を持ち主にできるのは
// 管理者だけで、管理者はもともとこのユーザーの鍵を読める。利用者が自分で動かす
// agent の pipe は、そのユーザー自身が持ち主になる。LocalService や
// NetworkService で動くサービスに鍵を預ける理由は無いので含めない。
func isTrustedPipeOwner(owner, currentUser *windows.SID) bool {
	if owner.IsWellKnown(windows.WinLocalSystemSid) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		return true
	}
	return currentUser != nil && owner.Equals(currentUser)
}
