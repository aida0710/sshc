package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// runStatus は、エンジンの様子をそのまま JSON で書き出す。
//
// **これは人のための表示ではない。** 読むのはメニューバーであり、だから
// 整形もしないし、翻訳もしない。エンジンが居なければ 1 で終わる。
func runStatus(
	ctx context.Context, stateDir string, client *http.Client, stdout, stderr io.Writer,
) int {
	answer, err := engineStatus(ctx, stateDir, client)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	encoded, err := json.Marshal(answer)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}

// engineStatus は、エンジンがいまどうなっているかを尋ねる。
func engineStatus(ctx context.Context, stateDir string, client *http.Client) (statusAnswer, error) {
	found, err := handoff.Read(stateDir)
	if err != nil {
		return statusAnswer{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		found.URL+httpserver.StatusPath, nil)
	if err != nil {
		return statusAnswer{}, err
	}
	request.Header.Set(handoff.HeaderName, found.Secret)

	response, err := client.Do(request)
	if err != nil {
		return statusAnswer{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return statusAnswer{}, fmt.Errorf("sshc refused the request")
	}
	var answer statusAnswer
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer); err != nil {
		return statusAnswer{}, err
	}
	return answer, nil
}

// statusAnswer は、エンジンが答える「いまどうなっているか」である。
type statusAnswer struct {
	// Vault は、開けるべき錠がそもそも有るか。
	Vault    bool `json:"vault"`
	Unlocked bool `json:"unlocked"`
	Sessions int  `json:"sessions"`
}

// locked は、いま開けるべき錠が掛かったままかを尋ねる。尋ねられなければ、
// 施錠されていないものとして扱う——聞けないなら聞かずに繋ぐ経路へ任せる。
//
// **保管庫が無いなら施錠もされていない。** 無い錠の鍵を尋ねると、保管庫を一度も
// 作っていない利用者は接続のたびにマスターパスワードを訊かれる——答えようの
// ない問いであり、新規インストール直後の利用者は全員そこに居る。
func locked(ctx context.Context, stateDir string, client *http.Client) bool {
	status, err := engineStatus(ctx, stateDir, client)
	return err == nil && status.Vault && !status.Unlocked
}

// unlock は、答えられたマスターパスワードをエンジンへ渡す。
//
// **開いたかどうかしか返らない。** どう間違っていたかを、この経路は言わない。
func unlock(ctx context.Context, stateDir string, client *http.Client, passphrase string) bool {
	found, err := handoff.Read(stateDir)
	if err != nil {
		return false
	}
	body, err := json.Marshal(map[string]string{"passphrase": passphrase})
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		found.URL+httpserver.UnlockPath, bytes.NewReader(body))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, found.Secret)

	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode == http.StatusNoContent
}
