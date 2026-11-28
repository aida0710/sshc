package application

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMovePreviewShowsEveryAliasTheMovedBlockDeclares(t *testing.T) {
	service, workspace := newTestService(t)
	const config = "Include conf.d/*.conf\n\nHost r100 mirage\n\tUser ops\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	preview, err := service.Preview(EditRequest{
		Kind: EditMove, Path: "config", Base: config, Alias: "r100",
		DestinationPath: "conf.d/10-home.conf", DestinationBase: readFile(t, workspace, "conf.d/10-home.conf"),
	})
	if err != nil {
		t.Fatal(err)
	}
	userSourceAfterMove := map[string]string{}
	for _, diff := range preview.Effective {
		for _, change := range diff.Changes {
			if change.Keyword == "User" {
				userSourceAfterMove[diff.Alias] = change.AfterSources[0].Path
			}
		}
	}
	want := map[string]string{"r100": "conf.d/10-home.conf", "mirage": "conf.d/10-home.conf"}
	if !reflect.DeepEqual(userSourceAfterMove, want) {
		t.Fatalf("file that sets User after the move by alias = %v, want %v", userSourceAfterMove, want)
	}
}

func TestMovePreviewIntoAGroupShowsTheGroupSettings(t *testing.T) {
	service, workspace := newTestService(t)
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"),
		[]byte("Include conf.d/*.conf\n\nHost r100\n\tUser ops\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	groups := []GroupMetadata{{Name: "work", Settings: []Setting{{Keyword: "Port", Values: []string{"2222"}}}}}
	if _, err := service.Save(EditRequest{Kind: EditGroups, Groups: groups}); err != nil {
		t.Fatal(err)
	}

	preview, err := service.Preview(EditRequest{
		Kind: EditMove, Path: "config", Base: readFile(t, workspace, "config"), Alias: "r100", DestinationGroup: "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Effective) != 1 || preview.Effective[0].Alias != "r100" {
		t.Fatalf("effective preview = %#v, want one diff for r100", preview.Effective)
	}
	// グループの設定は、保存のときに書き直すグループ設定のファイルから来る。
	for _, change := range preview.Effective[0].Changes {
		if change.Keyword == "Port" && strings.Join(change.After, " ") == "2222" {
			return
		}
	}
	t.Fatalf("effective changes = %#v, want Port 2222 from the group", preview.Effective[0].Changes)
}

func TestRenameAndDuplicatePreviewsCompareTheOldAliasWithTheNewAlias(t *testing.T) {
	const config = "Host r100\n\tHostName r100.example\n\tUser ops\n"
	for _, kind := range []EditKind{EditRename, EditDuplicate} {
		t.Run(string(kind), func(t *testing.T) {
			service, workspace := newTestService(t)
			if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}

			preview, err := service.Preview(EditRequest{
				Kind: kind, Path: "config", Base: config, Alias: "r100", NewAlias: "pelican",
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Effective) != 1 || preview.Effective[0].Alias != "pelican" {
				t.Fatalf("effective preview = %#v, want one diff for pelican", preview.Effective)
			}
			// 前の値を新しい接続名で読むと、まだ無い Host なのでどれも未設定になる。
			valueBeforeSave := map[string]string{}
			for _, change := range preview.Effective[0].Changes {
				valueBeforeSave[change.Keyword] = strings.Join(change.Before, " ")
			}
			if want := map[string]string{"HostName": "r100.example", "User": "ops"}; !reflect.DeepEqual(valueBeforeSave, want) {
				t.Fatalf("value before the save by keyword = %v, want the values of r100 %v", valueBeforeSave, want)
			}
		})
	}
}
