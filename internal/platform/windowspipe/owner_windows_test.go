//go:build windows

package windowspipe

import (
	"io"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func wellKnownSID(t *testing.T, kind windows.WELL_KNOWN_SID_TYPE) *windows.SID {
	t.Helper()
	sid, err := windows.CreateWellKnownSid(kind)
	if err != nil {
		t.Fatalf("create well-known SID %d: %v", kind, err)
	}
	return sid
}

func currentUserSID(t *testing.T) *windows.SID {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return user.User.Sid
}

// 鍵を預けてよいのは、ssh-agent サービス（LocalSystem）、管理者、自分の pipe だけである。
func TestPipesOwnedByLocalSystemAdministratorsOrTheCurrentUserAreTrusted(t *testing.T) {
	currentUser := currentUserSID(t)
	for name, owner := range map[string]*windows.SID{
		"LocalSystem":    wellKnownSID(t, windows.WinLocalSystemSid),
		"Administrators": wellKnownSID(t, windows.WinBuiltinAdministratorsSid),
		"current user":   currentUser,
	} {
		if !isTrustedPipeOwner(owner, currentUser) {
			t.Errorf("a pipe owned by %s (%s) was refused", name, owner)
		}
	}
}

// 同じマシンの別のユーザーが先に作った pipe へは、鍵を渡さない。
func TestPipesOwnedByAnotherUserOrAServiceAccountAreRefused(t *testing.T) {
	anotherUser, err := windows.StringToSid("S-1-5-21-1111111111-2222222222-3333333333-1001")
	if err != nil {
		t.Fatal(err)
	}
	currentUser := currentUserSID(t)
	if anotherUser.Equals(currentUser) {
		t.Fatal("the fixture SID collides with the current user")
	}
	for name, owner := range map[string]*windows.SID{
		"another user":    anotherUser,
		"Everyone":        wellKnownSID(t, windows.WinWorldSid),
		"Users":           wellKnownSID(t, windows.WinBuiltinUsersSid),
		"LocalService":    wellKnownSID(t, windows.WinLocalServiceSid),
		"NetworkService":  wellKnownSID(t, windows.WinNetworkServiceSid),
		"Authenticated":   wellKnownSID(t, windows.WinAuthenticatedUserSid),
		"Interactive":     wellKnownSID(t, windows.WinInteractiveSid),
		"Anonymous logon": wellKnownSID(t, windows.WinAnonymousSid),
	} {
		if isTrustedPipeOwner(owner, currentUser) {
			t.Errorf("a pipe owned by %s (%s) was trusted", name, owner)
		}
	}
}

// 自分が作った pipe は持ち主の確認を通り、そのまま繋がる。
func TestDialingAPipeThisUserCreatedPassesTheOwnerCheck(t *testing.T) {
	server := newPipeServer(t)
	accepting := server.accepting()
	client := dial(t, server)
	serverSide(t, accepting)

	if err := verifyPipeOwner(client.(*conn).handle); err != nil {
		t.Fatalf("the owner check refused a pipe this user created: %v", err)
	}
}

var (
	advapi32                   = windows.NewLazySystemDLL("advapi32.dll")
	impersonateNamedPipeClient = advapi32.NewProc("ImpersonateNamedPipeClient")
)

// pipe を作った側は、繋いできた利用者が誰かを確かめられるだけで、その利用者として
// 振る舞えない。振る舞えるなら、鍵を受け取る前に利用者のファイルを開ける。
func TestThePipeServerCanOnlyIdentifyTheClient(t *testing.T) {
	server := newPipeServer(t)
	accepting := server.accepting()
	client := dial(t, server)
	side := serverSide(t, accepting)

	// ImpersonateNamedPipeClient は、pipe から一度読んだあとでなければ失敗する。
	if _, err := client.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(side, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if succeeded, _, err := impersonateNamedPipeClient.Call(uintptr(side.(*conn).handle)); succeeded == 0 {
		t.Fatalf("impersonate the pipe client: %v", err)
	}
	defer func() {
		if err := windows.RevertToSelf(); err != nil {
			t.Fatalf("revert to self: %v", err)
		}
	}()
	var token windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token); err != nil {
		t.Fatalf("open the impersonation token: %v", err)
	}
	defer token.Close()
	var level uint32
	var returned uint32
	if err := windows.GetTokenInformation(token, windows.TokenImpersonationLevel, (*byte)(unsafe.Pointer(&level)), uint32(unsafe.Sizeof(level)), &returned); err != nil {
		t.Fatalf("read the impersonation level: %v", err)
	}
	if level != windows.SecurityIdentification {
		t.Errorf("impersonation level = %d, want SecurityIdentification (%d)", level, windows.SecurityIdentification)
	}
}
