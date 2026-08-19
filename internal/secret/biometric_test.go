package secret_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/secret"
)

// fakeGuardian は、OS の錠前の代わりである。**本物の Keychain 無しで、この機能の
// ほとんど全部が確かめられる**のは、番人が interface だからである。
type fakeGuardian struct {
	available bool
	kept      []byte
	refuse    bool
	reveals   int
	keeps     int
	forgets   int
}

func (g *fakeGuardian) Available() bool { return g.available }

func (g *fakeGuardian) Keep(secret []byte) error {
	g.keeps++
	g.kept = append([]byte(nil), secret...)
	return nil
}

func (g *fakeGuardian) Reveal() ([]byte, error) {
	g.reveals++
	if g.refuse {
		return nil, secret.ErrRefused
	}
	if g.kept == nil {
		return nil, secret.ErrNoBiometric
	}
	return append([]byte(nil), g.kept...), nil
}

func (g *fakeGuardian) Forget() error {
	g.forgets++
	g.kept = nil
	return nil
}

func enabledService(t *testing.T) (*secret.Service, *fakeGuardian, string) {
	t.Helper()
	service, home := newService(t)
	guardian := &fakeGuardian{available: true}
	service.SetGuardian(guardian)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.EnableBiometric(); err != nil {
		t.Fatalf("EnableBiometric = %v", err)
	}
	return service, guardian, home
}

// **これがこの機能そのものである。** 錠前が預かっているものだけで保管庫が開き、
// マスターパスワードは一度も通らない。
func TestTheKeptSecretOpensTheVaultWithoutThePassword(t *testing.T) {
	service, guardian, _ := enabledService(t)
	service.Lock()
	if service.Unlocked() {
		t.Fatal("the vault is still open")
	}

	if err := service.UnlockWithBiometric(); err != nil {
		t.Fatalf("UnlockWithBiometric = %v", err)
	}
	if !service.Unlocked() {
		t.Fatal("the vault did not open")
	}
	if guardian.reveals != 1 {
		t.Fatalf("the guardian was asked %d times", guardian.reveals)
	}
	// 保管庫の中身も、パスワードで開けたときと同じものが読める。
	if err := service.Set("bastion", "a password"); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if err := service.UnlockWithBiometric(); err != nil {
		t.Fatal(err)
	}
	if got := service.PasswordFor("bastion"); got != "a password" {
		t.Fatalf("stored password = %q", got)
	}
}

// **預けるのはマスターパスワードではない。** 錠前が破れた日に失うものを、この
// 保管庫だけに閉じ込めておくための約束である。
func TestWhatIsKeptIsNotTheMasterPassword(t *testing.T) {
	_, guardian, _ := enabledService(t)
	if len(guardian.kept) == 0 {
		t.Fatal("nothing was kept")
	}
	if string(guardian.kept) == passphrase {
		t.Fatal("the master password itself was handed to the operating system")
	}
}

// 断られたら、そこで終わりである。**失敗ではない**ので、パスワードの道は生きて
// いなければならない。
func TestARefusedProofLeavesThePasswordDoorOpen(t *testing.T) {
	service, guardian, _ := enabledService(t)
	service.Lock()
	guardian.refuse = true

	if err := service.UnlockWithBiometric(); !errors.Is(err, secret.ErrRefused) {
		t.Fatalf("UnlockWithBiometric = %v, want ErrRefused", err)
	}
	if service.Unlocked() {
		t.Fatal("the vault opened even though the person was refused")
	}
	if err := service.Unlock(passphrase); err != nil {
		t.Fatalf("the password no longer opens the vault: %v", err)
	}
}

// 保管庫が閉じているあいだは預けられない。開けられるなら、それは「パスワードを
// 知らなくても入口を作れる」ということである。
func TestABiometricEntranceCannotBeMadeWhileTheVaultIsShut(t *testing.T) {
	service, _ := newService(t)
	service.SetGuardian(&fakeGuardian{available: true})
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()

	if err := service.EnableBiometric(); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("EnableBiometric on a locked vault = %v, want ErrLocked", err)
	}
}

// 錠前の無い端末では、何も起きない。押せないものを見せないための答えでもある。
func TestWithoutALockNothingIsKept(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if state := service.Biometric(); state.Available || state.Enabled {
		t.Fatalf("state = %+v on a machine with no lock", state)
	}
	if err := service.EnableBiometric(); !errors.Is(err, secret.ErrNoGuardian) {
		t.Fatalf("EnableBiometric = %v, want ErrNoGuardian", err)
	}
}

// **マスターパスワードを変えたら、二つ目の入口は開かない。** 古い鍵を封じたもの
// だからである。黙って残せば、次の起動で理由の分からない失敗になる。
func TestChangingTheMasterPasswordTakesTheEntranceAway(t *testing.T) {
	service, guardian, home := enabledService(t)

	if err := service.ChangeMasterPassword(passphrase, "another master password"); err != nil {
		t.Fatalf("ChangeMasterPassword = %v", err)
	}
	if state := service.Biometric(); state.Enabled {
		t.Fatal("the entrance survived a change of key")
	}
	if guardian.forgets == 0 {
		t.Fatal("the operating system was left holding a secret that opens nothing")
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", filepath.FromSlash(secret.BiometricPath))); !os.IsNotExist(err) {
		t.Fatalf("the biometric file is still there: %v", err)
	}

	service.Lock()
	if err := service.UnlockWithBiometric(); !errors.Is(err, secret.ErrNoBiometric) {
		t.Fatalf("UnlockWithBiometric = %v, want ErrNoBiometric", err)
	}
	if err := service.Unlock("another master password"); err != nil {
		t.Fatalf("the new password does not open the vault: %v", err)
	}
}

// 錠前の中身が別のものに変わっていたら（別の端末の預かり、作り直された鍵）、
// 預かりごと捨てて、パスワードへ返す。
func TestASecretThatOpensNothingIsThrownAway(t *testing.T) {
	service, guardian, _ := enabledService(t)
	service.Lock()
	guardian.kept = []byte("a secret that was never wrapped around this key")

	if err := service.UnlockWithBiometric(); !errors.Is(err, secret.ErrNoBiometric) {
		t.Fatalf("UnlockWithBiometric = %v, want ErrNoBiometric", err)
	}
	if state := service.Biometric(); state.Enabled {
		t.Fatal("an entrance that opens nothing was kept")
	}
}

// 無効にすれば、両側から消える。
func TestForgettingTakesBothSidesAway(t *testing.T) {
	service, guardian, home := enabledService(t)
	if err := service.ForgetBiometric(); err != nil {
		t.Fatalf("ForgetBiometric = %v", err)
	}
	if guardian.kept != nil {
		t.Fatal("the operating system is still holding it")
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", filepath.FromSlash(secret.BiometricPath))); !os.IsNotExist(err) {
		t.Fatalf("the file is still there: %v", err)
	}
	if state := service.Biometric(); state.Enabled {
		t.Fatalf("state = %+v", state)
	}
}
