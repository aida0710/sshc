package vpn

import "fmt"

// 利用者が書いた設定ファイル（OpenVPN の .ovpn、WireGuard の設定ファイル）の中の、
// 受け取れない行を表す。どの backend でも、何行目のどの指示かを添えて返す。

// 設定ファイルを受け取れない理由のうち、どの backend でも使う語である。画面と CLI が翻訳する。
const (
	// ReasonRunsCommand は、コマンドやプログラムを実行する指示であることを表す。
	ReasonRunsCommand Reason = "runs_command"
	// ReasonChangesRoutes は、経路や DNS を変える指示であることを表す。
	ReasonChangesRoutes Reason = "changes_routes"
	// ReasonDecidedBySshc は、sshc が決める指示であることを表す。
	ReasonDecidedBySshc Reason = "decided_by_sshc"
	// ReasonConfigMismatch は、プロファイルに書いた値が、設定ファイルの中身と合わないことを
	// 表す（OpenVPN のサーバー、WireGuard の DNS）。
	ReasonConfigMismatch Reason = "config_mismatch"
)

// ConfigLineError は、設定ファイルの中の、受け取れない行である。項目の誤りに、何行目の
// どの指示かを添える。
type ConfigLineError struct {
	FieldError
	// Line は、1から数えた行番号である。ファイル全体についての誤りなら 0。
	Line int
	// Directive は、断った指示の名前である。backend の表にある名前だけを入れる。設定
	// ファイルはシークレットを含みうるので、表に無い語は応答に載せない。
	Directive string
}

func (failure *ConfigLineError) Error() string {
	return fmt.Sprintf("%s: line %d: %s", failure.FieldError.Error(), failure.Line, failure.Directive)
}

func (failure *ConfigLineError) Unwrap() error { return &failure.FieldError }

// configLine は、受け取れない行ひとつぶんの中身である。
type configLine struct {
	kind      error
	field     string
	reason    Reason
	line      int
	directive string
	// limit は、too_long と too_many などの上限である。無ければ 0。
	limit int
}

func newConfigLineError(refused configLine) *ConfigLineError {
	return &ConfigLineError{
		FieldError: FieldError{Kind: refused.kind, Field: refused.field, Reason: refused.reason, Limit: refused.limit},
		Line:       refused.line, Directive: refused.directive,
	}
}
