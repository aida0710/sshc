package effective

import "sshc/internal/config"

// hostBlockNotes は、Host ブロックが alias に一致したときに残す印を組み立てる。
//
// Resolve の Notes と Project の Complexities が同じ設定で同じ印を出すように、
// 印の種類と数え方をここ一か所に置く。片方だけを直すと、同じ alias について
// 二つの画面が違うことを言う。
type hostBlockNotes struct {
	alias string
	// declaring は、ここまでに alias をそのまま名指ししたブロックの数である。
	declaring int
}

// observe は、alias に一致した Host ブロックひとつについての印を返す。
func (notes *hostBlockNotes) observe(header blockHeader, kind string) []Complexity {
	at := func(code, detail string) Complexity {
		return Complexity{Code: code, Path: header.path, Line: header.number(), Condition: header.condition, Detail: detail}
	}
	var observed []Complexity
	// 数えるのは alias を指定しているブロックだけである。たまたま一致した
	// catch-all は「二つのブロックがこの名前を主張している」ではない。それは
	// ワイルドカードで一致したという別の話である。
	if DeclaresExactly(header.block.Patterns, notes.alias) {
		notes.declaring++
		if notes.declaring > 1 {
			observed = append(observed, at(ComplexityDuplicateAlias, "more than one Host block claims this alias"))
		}
	}
	if kind == SourceWildcard {
		observed = append(observed, at(ComplexityWildcardPattern, "this block matched through a wildcard pattern"))
	}
	for _, pattern := range header.block.Patterns {
		if pattern.Negated {
			observed = append(observed, at(ComplexityNegatedPattern, "this block excludes hosts through "+pattern.Raw))
			break
		}
	}
	return observed
}

// unresolvedIncludeNotes は、Include の診断のうち読み手が知っておくべきものを印にする。
//
// 読めない Include は拒否ではなく印である。読めた範囲で解決する。拒否にすると、
// まだ作られていないディレクトリを Include が指している間、その alias を解決
// できなくなる。グループを作る保存はまさにその状態を通る。
func unresolvedIncludeNotes(diagnostics []config.Diagnostic) []Complexity {
	var notes []Complexity
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == config.SeverityInfo {
			continue
		}
		notes = append(notes, Complexity{
			Code: ComplexityUnresolvedInclude, Path: diagnostic.Path,
			Line: diagnostic.Line, Detail: diagnostic.Code,
		})
	}
	return notes
}
