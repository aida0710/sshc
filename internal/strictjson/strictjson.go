// Package strictjson は、決まった形の JSON を 1 つの規則で読む。
//
// このアプリケーションが自分で書いた状態ファイル、engine と CLI のあいだの応答、
// 画面からの要求は、どれも形が決まっている。読むのは 1 つの文書だけで、未知の
// フィールドも、文書のあとに続くデータも断る。未知のフィールドを許すと、タイプミスや
// 別の版の項目が黙って捨てられ、続くデータを許すと、壊れたファイルを正しいものとして
// 読んでしまう。
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ErrTrailingData は、1 つ目の文書のあとに、空白以外のデータが続いていたことを表す。
var ErrTrailingData = errors.New("trailing data after the JSON document")

// Decode は、document を 1 つの JSON 文書として target へ読む。
//
// 文書として読めなければ encoding/json のエラーを、あとにデータが続けば
// ErrTrailingData を返す。呼び出し側は、自分の層の用語のエラーで包み直す。
func Decode(document []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrTrailingData
	}
	return nil
}
