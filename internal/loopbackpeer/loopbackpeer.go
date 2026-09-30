// Package loopbackpeer は、ループバックの TCP 接続を張った相手がどの OS ユーザーかを確かめる。
//
// Linux では、同じマシンの別 OS ユーザーも /proc/<pid>/cmdline を読める。ブラウザへ
// 渡した bootstrap URL は xdg-open とブラウザの引数に載るので、別 OS ユーザーが
// 読んで、正規のブラウザより先に使える。bootstrap を受け付ける前に接続の持ち主を
// 確かめ、読まれても使わせない。
//
// macOS と Windows は別ユーザーのプロセスの引数を読ませないので、確かめない。
// Android はアプリごとに uid が違い、ブラウザもエンジンとは別の uid で動くので、
// 確かめると正規のブラウザを拒んでしまう。
package loopbackpeer
