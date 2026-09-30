package windowsacl

import "testing"

// 実在の SID の形をした値を使う。判定は文字列の一致だけを見る。
const (
	thisUserSID       = "S-1-5-21-1000-1000-1000-1001"
	anotherAccountSID = "S-1-5-21-1000-1000-1000-1002"
	systemSID         = "S-1-5-18"
	administratorsSID = "S-1-5-32-544"
	everyoneSID       = "S-1-1-0"
)

var notOthersForThisUser = []string{thisUserSID, systemSID, administratorsSID}

func allowRead(trustee string) accessEntry {
	return accessEntry{kind: accessAllowed, grantsRead: true, trustee: trustee}
}

// 所有者は DACL に関係なく自分に読み取りを足せる。別のアカウントが所有する鍵は、
// そのアカウントに読み取りの ACE があっても無くても露出している。
func TestAKeyOwnedByAnotherAccountIsExposed(t *testing.T) {
	for name, entries := range map[string][]accessEntry{
		"with its read entry":          {allowRead(thisUserSID), allowRead(anotherAccountSID)},
		"without any entry of its own": {allowRead(thisUserSID)},
	} {
		t.Run(name, func(t *testing.T) {
			security := keySecurity{owner: anotherAccountSID, hasDACL: true, entries: entries}

			exposed, err := readableByOthers(security, notOthersForThisUser)

			if err != nil {
				t.Fatalf("readableByOthers: %v", err)
			}
			if !exposed {
				t.Error("a key owned by another account was reported as closed")
			}
		})
	}
}

// 昇格したトークンが作った鍵の所有者は Administrators になる。読み取りが本人と
// SYSTEM だけなら、それは閉じた鍵である。
func TestAKeyOwnedByAdministratorsAndReadOnlyByThisUserIsClosed(t *testing.T) {
	security := keySecurity{
		owner:   administratorsSID,
		hasDACL: true,
		entries: []accessEntry{allowRead(thisUserSID), allowRead(systemSID)},
	}

	exposed, err := readableByOthers(security, notOthersForThisUser)

	if err != nil {
		t.Fatalf("readableByOthers: %v", err)
	}
	if exposed {
		t.Error("a key an elevated token wrote for this user was reported as exposed")
	}
}

// 所有者の無い記述子は、閉じていると確かめられない。
func TestAKeyWithoutAnOwnerIsExposed(t *testing.T) {
	security := keySecurity{hasDACL: true, entries: []accessEntry{allowRead(thisUserSID)}}

	exposed, err := readableByOthers(security, notOthersForThisUser)

	if err != nil {
		t.Fatalf("readableByOthers: %v", err)
	}
	if !exposed {
		t.Error("a key whose descriptor names no owner was reported as closed")
	}
}

// 拒否を引き算しない。順序の壊れた DACL では許可が先に効くので、拒否があっても
// 読める相手への許可は露出である。
func TestADenyEntryDoesNotCancelAnotherTrusteesRead(t *testing.T) {
	security := keySecurity{
		owner:   thisUserSID,
		hasDACL: true,
		entries: []accessEntry{
			{kind: accessDenied, grantsRead: true, trustee: everyoneSID},
			allowRead(everyoneSID),
		},
	}

	exposed, err := readableByOthers(security, notOthersForThisUser)

	if err != nil {
		t.Fatalf("readableByOthers: %v", err)
	}
	if !exposed {
		t.Error("a deny entry was subtracted from another trustee's read")
	}
}

// 読み方を持たない種類の ACE があれば、閉じていると言わずに理由を返す。
func TestAnUnreadableEntryIsReportedAsExposedWithAReason(t *testing.T) {
	security := keySecurity{
		owner:   thisUserSID,
		hasDACL: true,
		entries: []accessEntry{{kind: accessUnreadable, aceType: 5}},
	}

	exposed, err := readableByOthers(security, notOthersForThisUser)

	if err == nil {
		t.Fatal("an access entry sshc cannot read was judged without a reason")
	}
	if !exposed {
		t.Error("an access entry sshc cannot read was reported as closed")
	}
}
