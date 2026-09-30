package main

// sshc の CLI が返す終了コード。成功は 0 である。値は、スクリプトが見分けるための
// 公開の約束なので、変えない。
const (
	// exitFailure は、操作が失敗したことを表す。
	exitFailure = 1
	// exitUsage は、引数の形が合わないことを表す。多くの CLI が usage の誤りに使う値。
	exitUsage = 2
	// exitTimeout は、指定された時間内に終わらなかったことを表す。timeout(1) と同じ値。
	exitTimeout = 124
	// exitInterrupted は、利用者の Ctrl-C で止めたことを表す。シェルが SIGINT（2）で
	// 終わったプロセスに付ける 128+2 と同じ値にする。
	exitInterrupted = 130
)
