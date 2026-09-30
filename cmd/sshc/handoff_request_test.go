package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// handoff の秘密を付けた要求は、どの経路でも redirect を追わない。追えば、redirect 先へ
// X-SSHC-CLI がそのまま付いて届く。
func TestHandoffRequestsDoNotFollowRedirects(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(stateDir string, found handoff.Handoff, client *http.Client) bool
	}{
		{"open", func(stateDir string, _ handoff.Handoff, client *http.Client) bool {
			environment := commandEnvironment{stateDir: stateDir, client: client, stdout: io.Discard, stderr: io.Discard}
			return runOpen(context.Background(), environment, false) != 0
		}},
		{"connect", func(_ string, found handoff.Handoff, client *http.Client) bool {
			_, err := requestConnection(context.Background(), found, "server-a", client)
			return err != nil
		}},
		{"stop", func(stateDir string, found handoff.Handoff, client *http.Client) bool {
			err := stopRunningEngine(context.Background(), stateDir, found, client,
				func(string) (func() error, error) { return func() error { return nil }, nil })
			return err != nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var secretRedirected atomic.Bool
			target := engineTestServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				if request.Header.Get(handoff.HeaderName) != "" {
					secretRedirected.Store(true)
				}
			}))
			defer target.Close()
			server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				http.Redirect(response, request, target.URL+request.URL.Path, http.StatusTemporaryRedirect)
			}))
			defer server.Close()
			stateDir := t.TempDir()
			writeTestHandoff(t, stateDir, server.URL)

			if failed := test.call(stateDir, testHandoff(server.URL), server.Client()); !failed {
				t.Fatal("a redirect was accepted as the engine's answer")
			}
			if secretRedirected.Load() {
				t.Fatal("the handoff secret followed a redirect")
			}
		})
	}
}

// 接続情報の応答は engine の形どおりのときだけ読む。未知の項目や末尾の余りがあれば、
// 別のものが答えたとみなして使わない。
func TestConnectionAnswerIsReadOnlyInTheEngineShape(t *testing.T) {
	for _, test := range []struct {
		name   string
		answer string
		valid  bool
	}{
		{"engine shape", `{"alias":"server-a","warnings":[]}`, true},
		{"unknown field", `{"alias":"server-a","warnings":[],"surprise":true}`, false},
		{"trailing JSON", `{"alias":"server-a","warnings":[]}{}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(response, test.answer)
			}))
			defer server.Close()
			answer, err := requestConnection(context.Background(), testHandoff(server.URL), "server-a", server.Client())
			if test.valid && (err != nil || answer.Alias != "server-a") {
				t.Fatalf("requestConnection = %#v, %v", answer, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("requestConnection accepted %s", test.answer)
			}
		})
	}
}

// sshc ssh と sshc open は、sshcエンジンに断られたとき、断った理由の code を添えて伝える。
// 「sshc: refused」だけでは、どの断りも同じ一文になり、理由を追えない。sshc ssh は
// 状態と接続情報の 2 つを要求するので、どちらで断られても同じ文になることを確かめる。
func TestSSHAndOpenNameTheCodeTheEngineRefusedWith(t *testing.T) {
	refusals := []struct {
		name     string
		status   int
		body     string
		wantCode string
	}{
		// 止まる途中の sshcエンジンは、POST の要求を stoppingGate が problem で断る。
		{"problem", http.StatusServiceUnavailable, `{"code":"server_stopping","message":"request rejected"}`, "server_stopping"},
		// /cli/ のルートは本文の無い status で断る（Vault の状態を読めない /cli/status の 500 など）。
		// decodeEngineProblem はそれを http_error と読む。
		{"bare status", http.StatusInternalServerError, "", "http_error"},
	}
	commands := []struct {
		name string
		// paths は、コマンドが証明のあとに送る要求を、送る順に並べたものである。
		paths []string
		run   func(context.Context, commandEnvironment) int
	}{
		{"open", []string{httpserver.OpenPath}, func(ctx context.Context, environment commandEnvironment) int {
			return runOpen(ctx, environment, false)
		}},
		{"ssh", []string{httpserver.StatusPath, httpserver.ConnectPath}, func(ctx context.Context, environment commandEnvironment) int {
			return runConnect(ctx, "server-a", environment)
		}},
		{"ssh --non-interactive", []string{httpserver.StatusPath, httpserver.ConnectPath}, func(ctx context.Context, environment commandEnvironment) int {
			return runRemote(ctx, "server-a", "true", environment)
		}},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			for _, command := range commands {
				t.Run(command.name, func(t *testing.T) {
					for _, refusedPath := range command.paths {
						t.Run("refused at "+strings.TrimPrefix(refusedPath, "/cli/"), func(t *testing.T) {
							server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
								switch request.URL.Path {
								case refusedPath:
									if refusal.body != "" {
										response.Header().Set("Content-Type", "application/problem+json")
									}
									response.WriteHeader(refusal.status)
									_, _ = io.WriteString(response, refusal.body)
								case httpserver.StatusPath:
									response.Header().Set("Content-Type", "application/json")
									_, _ = io.WriteString(response, validEngineStatus())
								default:
									t.Errorf("the command sent %s after it was refused", request.URL.Path)
									response.WriteHeader(http.StatusNotFound)
								}
							}))
							defer server.Close()

							stateDir := t.TempDir()
							writeTestHandoff(t, stateDir, server.URL)
							var stderr strings.Builder
							code := command.run(context.Background(), commandEnvironment{
								home: t.TempDir(), stateDir: stateDir, client: server.Client(), stdout: io.Discard, stderr: &stderr,
							})
							if code == 0 {
								t.Fatal("a refused request exited 0")
							}
							want := "sshc: the engine refused the request (code " + refusal.wantCode + ")\n"
							if got := stderr.String(); got != want {
								t.Fatalf("stderr = %q, want %q", got, want)
							}
						})
					}
				})
			}
		})
	}
}
