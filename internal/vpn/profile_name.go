package vpn

import (
	"unicode"
	"unicode/utf8"
)

// プロファイル名の規則。
//
// 名前は利用者が見分けるための表示名で、空白や日本語も使える。コンテナ名、経路の
// 置き場所、ソケットのパスには、名前から決まる識別子（profileIdentifier）を使うので、
// 名前の字はそれらの制約を受けない。

// maxProfileNameLength は、プロファイル名の上限（文字数）である。画面の一覧と CLI の
// 一覧の1行に収まる長さにする。API（api/openapi.yaml の VPNProfile.name）と同じ値に
// する。ここより長い名前を CLI から保存できると、画面が一覧の応答ごと受け取れなくなる。
const maxProfileNameLength = 48

// ValidateName は、プロファイル名として使えるかを確かめる。
//
// 保存する側も同じ規則で確かめる。
func ValidateName(name string) error { return validateProfileName(name) }

// validateProfileName は、印字できる字だけの名前を通す。
//
// 制御文字と、見た目に現れない書式の字（向きを変える字など）は断る。ターミナルや
// 接続ログで、別の名前に見せかけられないためである。`/` と `\` は、名前が API の
// パスの1区切り（/api/v1/vpn/profiles/{name}）に入るので断る。途中でパスの区切りと
// 読まれうる。前後の空白は、見た目では同じ名前が2つできるので断る。
func validateProfileName(name string) error {
	if name == "" {
		return fieldError(ErrProfileName, "name", ReasonRequired)
	}
	if utf8.RuneCountInString(name) > maxProfileNameLength {
		return &FieldError{Kind: ErrProfileName, Field: "name", Reason: ReasonTooLong, Limit: maxProfileNameLength}
	}
	if !utf8.ValidString(name) {
		return fieldError(ErrProfileName, "name", ReasonFormat)
	}
	for _, character := range name {
		if !unicode.IsGraphic(character) || character == '/' || character == '\\' {
			return fieldError(ErrProfileName, "name", ReasonFormat)
		}
	}
	first, _ := utf8.DecodeRuneInString(name)
	last, _ := utf8.DecodeLastRuneInString(name)
	if unicode.IsSpace(first) || unicode.IsSpace(last) {
		return fieldError(ErrProfileName, "name", ReasonFormat)
	}
	return nil
}
