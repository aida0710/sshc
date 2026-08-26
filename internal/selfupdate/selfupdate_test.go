package selfupdate_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sshc/internal/selfupdate"
)

// より新しいバージョンとは、数値として比較して後のものである。文字列として比較
// すると 0.10.0 は 0.9.0 より古くなり、それを更新として提示すればユーザーを後ろへ
// 歩かせることになる。
func TestNewerComparesNumbersAndNotText(t *testing.T) {
	for _, test := range []struct {
		current, candidate string
		want               bool
	}{
		{"0.9.0", "0.10.0", true},
		{"0.10.0", "0.9.0", false},
		{"1.0.0", "1.0.0", false},
		{"v1.2.3", "v1.2.4", true},
		{"1.2.3", "1.2.3", false},
		{"v1.2.3-beta.2", "v1.2.3-beta.10", true},
		{"v1.2.3", "v1.2.3-beta.10", false},
		// リリースでないビルドには、リリースがあることは伝え、どれだけ遅れているかは
		// 決して伝えない。
		{"dev", "0.1.0", true},
		// ネットワークから来た不正なtagを更新として扱わない。
		{"0.1.0", "dev", false},
		{"0.1.0", "definitely-not-a-release", false},
	} {
		if got := selfupdate.Newer(test.current, test.candidate); got != test.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", test.current, test.candidate, got, test.want)
		}
	}
}

func TestStableTagOnlyAcceptsCanonicalStableReleases(t *testing.T) {
	for _, test := range []struct {
		value string
		want  string
		ok    bool
	}{
		{"v1.2.3", "v1.2.3", true},
		{"1.2.3", "v1.2.3", true},
		{"v1.2.3-rc.1", "", false},
		{"v1.2.3+build", "", false},
		{"latest", "", false},
		{"v01.2.3", "", false},
	} {
		got, ok := selfupdate.StableTag(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("StableTag(%q) = %q, %v; want %q, %v", test.value, got, ok, test.want, test.ok)
		}
	}
}

// serve は偽の GitHub。どのテストも本物には到達しない。
func serve(t *testing.T, handler http.HandlerFunc) selfupdate.Checker {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/releases/latest", handler)
	return selfupdate.Checker{API: server.URL + "/releases/latest", HTTP: server.Client()}
}

func TestLatestReportsTheTagAndWhereToReadAboutIt(t *testing.T) {
	checker := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.2.0",
			"html_url": "https://example.invalid/releases/v0.2.0",
		})
	})

	found, err := checker.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest = %v", err)
	}
	if found.Version != "v0.2.0" || found.PageURL != "https://example.invalid/releases/v0.2.0" {
		t.Errorf("Latest = %+v", found)
	}
}

func TestLatestSaysWhenNothingIsPublished(t *testing.T) {
	checker := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := checker.Latest(context.Background()); !errors.Is(err, selfupdate.ErrNoRelease) {
		t.Errorf("Latest = %v, want ErrNoRelease", err)
	}
}

// ドラフトは、API が何と呼ぼうと公開されたものではない。
func TestLatestIgnoresADraft(t *testing.T) {
	checker := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0", "draft": true})
	})
	if _, err := checker.Latest(context.Background()); !errors.Is(err, selfupdate.ErrNoRelease) {
		t.Errorf("Latest = %v, want ErrNoRelease", err)
	}
}
