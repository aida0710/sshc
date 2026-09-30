// Package sshmatch は、OpenSSH の match.c が実装するパターン一致を持つ。
//
// ssh_config の Host と Match、known_hosts のホスト欄、アルゴリズムの並びの
// 指定は、どれも同じ規則で一致を判定する。規則の修正（'!' の扱いなど）が
// 片方だけに入らないように、一か所に置く。
package sshmatch

import "strings"

// Case は、パターン列の一致で大文字小文字を区別するかである。
type Case int

const (
	// CaseSensitive は区別する。Host 行、Match user、Match localuser がこれである。
	CaseSensitive Case = iota
	// IgnoreCase は、パターンと値の ASCII 英字を小文字にしてから比べる。OpenSSH の
	// match_hostname（Match host と originalhost、known_hosts のホスト欄）がこれである。
	IgnoreCase
)

// Pattern は OpenSSH の match_pattern である。'*' は任意の並びに、'?' はちょうど
// 1 文字に一致し、他のメタ文字に特別な意味はない。大文字小文字を区別する。
func Pattern(pattern, value string) bool {
	patternIndex, valueIndex := 0, 0
	starIndex, resumeIndex := -1, 0
	for valueIndex < len(value) {
		switch {
		case patternIndex < len(pattern) &&
			(pattern[patternIndex] == '?' || pattern[patternIndex] == value[valueIndex]):
			patternIndex++
			valueIndex++
		case patternIndex < len(pattern) && pattern[patternIndex] == '*':
			starIndex = patternIndex
			resumeIndex = valueIndex
			patternIndex++
		case starIndex >= 0:
			patternIndex = starIndex + 1
			resumeIndex++
			valueIndex = resumeIndex
		default:
			return false
		}
	}
	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

// PatternList は OpenSSH の match_pattern_list である。パターンのどれかに一致すれば
// 真になる。'!' で始まるパターンに一致したら、ほかのパターンに一致していても偽になる。
// 空のパターンは読み飛ばす。
func PatternList(patterns []string, value string, sensitivity Case) bool {
	if sensitivity == IgnoreCase {
		value = LowerASCII(value)
	}
	matched := false
	for _, pattern := range patterns {
		if sensitivity == IgnoreCase {
			pattern = LowerASCII(pattern)
		}
		if negated, found := strings.CutPrefix(pattern, "!"); found {
			if Pattern(negated, value) {
				return false
			}
			continue
		}
		if pattern != "" && Pattern(pattern, value) {
			matched = true
		}
	}
	return matched
}

// LowerASCII は OpenSSH の lowercase() と同じく、ASCII の英字だけを小文字にする。
//
// strings.ToLower は全角の英字なども変える。OpenSSH は C のロケールで tolower を
// 使うので、それらは変わらない。同じ名前を別の名前に変えると、一致の判定と
// known_hosts の照合が ssh とずれる。
func LowerASCII(value string) string {
	for index := 0; index < len(value); index++ {
		if 'A' <= value[index] && value[index] <= 'Z' {
			lowered := []byte(value)
			for position := index; position < len(lowered); position++ {
				if 'A' <= lowered[position] && lowered[position] <= 'Z' {
					lowered[position] += 'a' - 'A'
				}
			}
			return string(lowered)
		}
	}
	return value
}
