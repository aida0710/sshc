package application

import (
	"errors"
	"fmt"
	"testing"
)

// commitVPNProfileChange は、計画した変更を metadata だけで書く。
func commitVPNProfileChange(t *testing.T, service *Service, change VPNProfileChange, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitVPNProfileChange(change, nil); err != nil {
		t.Fatal(err)
	}
}

// storedVPNProfileID は、保存済みのプロファイル name の識別子を返す。
func storedVPNProfileID(t *testing.T, service *Service, name string) string {
	t.Helper()
	stored, err := service.StoredVPNProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	return stored.ID
}

// 作ったプロファイルの識別子は sshc エンジンが決め、要求に書かれた値は使わない。名前の
// 変更と編集では変わらないので、割り当ての結び付けの値も変わらない。
func TestACreatedProfileGetsItsOwnIDWhichRenamingAndEditingKeep(t *testing.T) {
	service := serviceWithVPNMetadata(t, NewMetadata())
	requested := labProfile()
	requested.ID = "ffffffffffffffffffffffffffffffff"
	change, err := service.PlanVPNProfileCreate(requested)
	commitVPNProfileChange(t, service, change, err)
	created := storedVPNProfileID(t, service, "lab")
	if !validVPNProfileID(created) || created == requested.ID {
		t.Fatalf("created id = %q, want a new id rather than the requested %q", created, requested.ID)
	}

	change, err = service.PlanVPNProfileRename("lab", "lab 2")
	commitVPNProfileChange(t, service, change, err)
	edited := labProfile()
	edited.Name, edited.ID = "lab 2", "ffffffffffffffffffffffffffffffff"
	edited.WireGuard = &WireGuardProfile{Servers: []string{"other.vpn.example.jp"}}
	change, err = service.PlanVPNProfileUpdate(edited)
	commitVPNProfileChange(t, service, change, err)

	if kept := storedVPNProfileID(t, service, "lab 2"); kept != created {
		t.Fatalf("id after renaming and editing = %q, want %q", kept, created)
	}
}

// 削除した名前で作り直したプロファイルは、別の識別子を持つ。前のプロファイルの割り当てを
// 引き継がない。
func TestAProfileRecreatedUnderARemovedNameGetsAnotherID(t *testing.T) {
	service := serviceWithVPNMetadata(t, NewMetadata())
	change, err := service.PlanVPNProfileCreate(labProfile())
	commitVPNProfileChange(t, service, change, err)
	removed := storedVPNProfileID(t, service, "lab")

	change, err = service.PlanVPNProfileRemove("lab")
	commitVPNProfileChange(t, service, change, err)
	change, err = service.PlanVPNProfileCreate(labProfile())
	commitVPNProfileChange(t, service, change, err)

	if recreated := storedVPNProfileID(t, service, "lab"); recreated == removed {
		t.Fatalf("the recreated lab shares the id %q of the removed one", recreated)
	}
}

// schema 9 より前の metadata のプロファイルは、名前から決めた識別子を得る。書き直すまでは
// 読むたびに移行するので、何度読んでも同じ識別子になり、割り当ての結び付けの値が合う。
func TestProfilesFromAnOlderSchemaGetTheSameIDOnEveryRead(t *testing.T) {
	document := []byte(`{"schemaVersion":8,"vpnProfiles":[` +
		`{"name":"lab","backend":"wireguard","wireguard":{"servers":["lab.vpn.example.jp"]}},` +
		`{"name":"office","backend":"wireguard","wireguard":{"servers":["office.vpn.example.jp"]}}]}`)

	first, err := DecodeMetadata(document)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DecodeMetadata(document)
	if err != nil {
		t.Fatal(err)
	}

	for index, profile := range first.VPNProfiles {
		if !validVPNProfileID(profile.ID) || profile.ID != second.VPNProfiles[index].ID {
			t.Fatalf("%s: ids %q and %q, want the same valid id on every read", profile.Name, profile.ID, second.VPNProfiles[index].ID)
		}
	}
	if first.VPNProfiles[0].ID == first.VPNProfiles[1].ID {
		t.Fatalf("lab and office share the id %q", first.VPNProfiles[0].ID)
	}
	if _, err := EncodeMetadata(first); err != nil {
		t.Fatalf("EncodeMetadata = %v", err)
	}
}

// 今の形の metadata で、識別子の無いプロファイルと、識別子が重なるプロファイルは保存しない。
// どちらも、別のプロファイルを通る接続に割り当てが渡りうる。
func TestAProfileWithoutItsOwnIDIsNotSaved(t *testing.T) {
	withoutID := labProfile()
	withoutID.ID = ""
	office := labProfile()
	office.Name = "office"

	for situation, profiles := range map[string][]VPNProfile{
		"no id":       {withoutID},
		"a shared id": {labProfile(), office},
	} {
		metadata := NewMetadata()
		metadata.VPNProfiles = profiles
		if _, err := EncodeMetadata(metadata); !errors.Is(err, ErrMetadataVPN) {
			t.Errorf("%s: EncodeMetadata = %v, want ErrMetadataVPN", situation, err)
		}
	}
}

// 識別子は 32 桁の小文字の16進である。openapi.yaml の VPNProfile.id の pattern と同じ形。
func TestTheIDOfAProfileIsThirtyTwoLowercaseHexDigits(t *testing.T) {
	created, err := newVPNProfileID()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{created, migratedVPNProfileID("lab")} {
		if !validVPNProfileID(id) {
			t.Errorf("%q is not a valid id", id)
		}
	}
	for _, id := range []string{"", "0123456789ABCDEF0123456789ABCDEF", fmt.Sprintf("%031d", 0), fmt.Sprintf("%033d", 0)} {
		if validVPNProfileID(id) {
			t.Errorf("%q was accepted as an id", id)
		}
	}
}
