//go:build !windows

package config

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const testHome = "/Users/tester"

// testOutside は、ワークスペースの外にある絶対パスの起点。Windows では別の
// ボリュームでなければ「外」にならないので、これも OS ごとに書き分ける。
const testOutside = "/etc/ssh"
