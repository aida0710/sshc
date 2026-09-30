//go:build unix

package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"sshc/internal/api"
)

// 取り消しで読み取りを抜ける仕組み（poll とパイプ）は Unix にしかない。Windows の
// コンソールは Ctrl-C で行入力を戻すが、テストの匿名パイプではその動きを再現できない。

// canceledSetupWatchdog は、取り消しで読み取りが戻らなかったときにテストを止める上限。
// 正しく動けば取り消しの直後に戻るので、この時間を待つことはない。
const canceledSetupWatchdog = 10 * time.Second

func TestSyncSetupCancelReturnsWhileWaitingForVisibleInputWithoutEnter(t *testing.T) {
	answers := []string{
		"https://objects.example.test",
		"ssh-config",
		"team/hosts",
		"ap-northeast-1",
		"both",
	}
	labels := []string{"Endpoint [", "Bucket [", "Path [", "Region [", "Direction ["}
	for cancelAt := range answers {
		t.Run(answers[cancelAt], func(t *testing.T) {
			script, server, stateDir := newSyncSetupServer(t, api.Empty)
			defer server.Close()
			inputRead, inputWrite, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer inputRead.Close()
			defer inputWrite.Close()
			promptRead, promptWrite, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer promptRead.Close()
			// 前の質問には答え、この質問では何も打たずに（Enter も押さずに）取り消す。
			answered := strings.Join(answers[:cancelAt], "\n")
			if answered != "" {
				answered += "\n"
			}
			if _, err := inputWrite.WriteString(answered); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go cancelWhenPromptShows(promptRead, labels[cancelAt], cancel)

			terminal := &setupPasswordTerminal{answers: standardHiddenSetup(false), terminals: map[uintptr]bool{
				inputRead.Fd(): true, promptWrite.Fd(): true,
			}}
			codes := make(chan int, 1)
			go func() {
				defer promptWrite.Close()
				codes <- runSync(ctx, syncInvocation{Action: syncSetup}, commandEnvironment{
					stateDir: stateDir, client: server.Client(), stdin: inputRead,
					stdout: io.Discard, stderr: promptWrite, terminal: terminal,
				})
			}()
			select {
			case code := <-codes:
				if code != 130 || len(script.checkBodies) != 0 || len(script.completeBodies) != 0 || terminal.reads != 0 {
					t.Fatalf("code=%d check=%d complete=%d hiddenReads=%d",
						code, len(script.checkBodies), len(script.completeBodies), terminal.reads)
				}
			case <-time.After(canceledSetupWatchdog):
				t.Fatal("setup kept waiting for Enter after the context was canceled")
			}
		})
	}
}

// cancelWhenPromptShows は質問の文が端末に出た後で取り消す。出る前に取り消すと、
// 読み取りを待つ前の確認で止まり、待っている読み取りを抜けられるかを確かめられない。
func cancelWhenPromptShows(prompt io.Reader, label string, cancel context.CancelFunc) {
	var shown strings.Builder
	buffer := make([]byte, 256)
	for {
		count, err := prompt.Read(buffer)
		shown.Write(buffer[:count])
		if strings.Contains(shown.String(), label) {
			cancel()
			_, _ = io.Copy(io.Discard, prompt)
			return
		}
		if err != nil {
			return
		}
	}
}
