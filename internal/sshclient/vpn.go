package sshclient

import "errors"

// VPN 経由の接続は、この機械から出る最初のホップにだけ効く。
//
// 経路はプロファイルごとのコンテナの中にあり、engine はそのコンテナが差し出す
// 中継へ繋ぐ。ここにあるのは、その輸送を選ぶときに断る条件だけである。

// ErrVPNThroughJump は、踏み台の向こうのホップに VPN 指定がある設定を断る。
//
// 2 ホップ目以降は手前の SSH 接続の中を通るので、この機械の VPN を通る余地が
// ない。黙って無視すると、利用者は VPN を通っていると思ったまま別経路で繋ぐ。
var ErrVPNThroughJump = errors.New("a jump hop cannot be reached through a VPN profile")

// ErrVPNWithProxyCommand は、VPN と ProxyCommand の両方が指定された設定を断る。
//
// ProxyCommand はこの機械で走るので、VPN の中には居ない。どちらを使うかを
// 推測すると、利用者が思っている経路と違う方で繋ぎうる。
var ErrVPNWithProxyCommand = errors.New("a VPN profile and a ProxyCommand cannot both reach one host")

// ErrVPNUnavailable は、VPN を指定した接続を、その経路を作れない engine が
// 受け取ったことを表す。
var ErrVPNUnavailable = errors.New("this engine cannot open VPN routes")
