package vpnrefusal

import (
	"fmt"

	"sshc/internal/vpn"
)

// phrasebook は、ひとつの言語で理由を1文に組み立てるための言い方の表である。
// どの言語も同じ組み立て方（sentence）を使い、表と枠の文だけを差し替える。
type phrasebook struct {
	// codes は、理由の語を持たない拒否の言い方である。
	codes map[string]string
	// fields は、項目を受け取れなかった理由の言い方である。%d を含むものには上限が入る。
	fields map[vpn.Reason]string
	// destinations は、接続先を VPN 経由で使えない理由の言い方である。
	destinations map[vpn.Reason]string
	// routes は、経路を用意できなかった理由の言い方である。
	routes map[vpn.FailureReason]string
	// targets は、経路はあるが接続先へ繋げなかった理由の言い方である。
	targets map[vpn.FailureReason]string
	// directives は、設定ファイルの指示ひとつを断った理由の言い方である。%s には
	// 断った指示が入る。
	directives map[vpn.Reason]string
	frames     phraseFrames
}

// phraseFrames は、表の文を包む枠と、表に当たらなかったときの文である。
type phraseFrames struct {
	// routeFailed と targetFailed は、%s に理由の文が入る。
	routeFailed  string
	targetFailed string
	// operationFailed は、知らない拒否の文である。
	operationFailed string
	// secretsMissing と profileInvalid は、項目が分からない項目の誤りの文である。
	secretsMissing string
	profileInvalid string
	// lineDirective は、行番号（%d）と、指示を名指しした理由の文（%s）を並べる。
	lineDirective string
	// line は、行番号（%d）と、指示を名指ししない理由の文（%s）を並べる。
	line string
	// peerMissing は、行番号（%d）の [Peer] に指示（%s）が無いことを言う。
	peerMissing string
	// fileMissing は、設定ファイルに指示（%s）が無いことを言う。
	fileMissing string
	// fieldSeparator は、項目の誤りの項目名と理由の間に置く区切りである。画面の
	// vpn.fieldRefusedAt と同じく、日本語は全角の「：」、英語は「: 」にする。
	fieldSeparator string
}

// sentence は、理由を1文にする。
func (book phrasebook) sentence(refusal Refusal) string {
	switch refusal.Code {
	case CodeProfileInvalid, CodeSecretsMissing:
		return book.fieldSentence(refusal)
	case CodeDestinationInvalid:
		sentence, known := book.destinations[vpn.Reason(refusal.Reason)]
		if !known {
			sentence = book.destinations[vpn.ReasonFormat]
		}
		return sentence
	case CodeTargetFailed:
		sentence, known := book.targets[vpn.FailureReason(refusal.Reason)]
		if !known {
			sentence = book.routes[vpn.FailureUnknown]
		}
		return fmt.Sprintf(book.frames.targetFailed, sentence)
	case CodeRouteFailed:
		sentence, known := book.routes[vpn.FailureReason(refusal.Reason)]
		if !known {
			sentence = book.routes[vpn.FailureUnknown]
		}
		return fmt.Sprintf(book.frames.routeFailed, sentence)
	}
	if sentence, known := book.codes[refusal.Code]; known {
		return sentence
	}
	return book.frames.operationFailed
}

// fieldSentence は、項目の誤りを「項目」と「理由」を区切りでつないだ1文にする。項目が分からなければ、
// 何を確かめればよいかだけを言う。
func (book phrasebook) fieldSentence(refusal Refusal) string {
	sentence, known := book.fields[vpn.Reason(refusal.Reason)]
	if refusal.Field == "" || !known {
		if refusal.Code == CodeSecretsMissing {
			return book.frames.secretsMissing
		}
		return book.frames.profileInvalid
	}
	if refusal.Limit > 0 {
		sentence = fmt.Sprintf(sentence, refusal.Limit)
	}
	if refusal.Line > 0 || refusal.Directive != "" {
		sentence = book.configLineSentence(refusal, sentence)
	}
	return refusal.Field + book.frames.fieldSeparator + sentence
}

// configLineSentence は、設定ファイルの中の誤りを、行番号と指示を添えた文にする。
// sentence は、行を添えない場合の理由の文である。
func (book phrasebook) configLineSentence(refusal Refusal, sentence string) string {
	reason := vpn.Reason(refusal.Reason)
	if reason == vpn.ReasonMissingDirective && refusal.Directive != "" {
		if refusal.Line > 0 {
			// 行を添えて足りないと言うのは、[Peer] に PublicKey が無いときだけである。
			return fmt.Sprintf(book.frames.peerMissing, refusal.Line, refusal.Directive)
		}
		return fmt.Sprintf(book.frames.fileMissing, refusal.Directive)
	}
	if refusal.Line == 0 {
		return sentence
	}
	if pattern, known := book.directives[reason]; known && refusal.Directive != "" {
		return fmt.Sprintf(book.frames.lineDirective, refusal.Line, fmt.Sprintf(pattern, refusal.Directive))
	}
	return fmt.Sprintf(book.frames.line, refusal.Line, sentence)
}
