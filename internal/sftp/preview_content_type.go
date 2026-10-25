package sftp

import "bytes"

// previewContentType は、先頭のバイト列だけからその型を返す。preview で
// 描いてよい型でなければ空を返す。名前が名乗る型は使わない。拡張子は
// 中身について何も保証しないからで、background 画像と同じ扱いである。
//
// 画像だけである。PDF は <iframe> でしか描けず、それには CSP へ blob: を
// 足す必要がある。画像は data: URL で足りるので、policy はそのままでよい。
func previewContentType(head []byte) string {
	switch {
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "image/jpeg"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "image/webp"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return "image/gif"
	case len(head) >= 2 && head[0] == 'B' && head[1] == 'M':
		return "image/bmp"
	case startsWithSVGRoot(head):
		return "image/svg+xml"
	}
	return ""
}

// startsWithSVGRoot は、XML 宣言・コメント・DOCTYPE の後に最初に来る要素が
// <svg であるかを見る。SVG には固定の先頭バイトが無いので、根の要素で決める。
// 根が先頭の範囲に収まらない SVG は、テキストとして見せる。
//
// 画面は data: URL を <img> に入れて描くので、SVG の script は動かない。この
// 応答を直接開いた場合も、API の応答に付く CSP（script-src 'self'）と nosniff
// が中の script を止める。
func startsWithSVGRoot(head []byte) bool {
	rest := bytes.TrimPrefix(head, []byte("\xEF\xBB\xBF"))
	for {
		rest = bytes.TrimLeft(rest, " \t\r\n")
		switch {
		case bytes.HasPrefix(rest, []byte("<?")):
			rest = afterMarker(rest, "?>")
		case bytes.HasPrefix(rest, []byte("<!--")):
			rest = afterMarker(rest, "-->")
		case bytes.HasPrefix(rest, []byte("<!")):
			rest = afterMarker(rest, ">")
		default:
			return isSVGStartTag(rest)
		}
		if rest == nil {
			return false
		}
	}
}

// afterMarker は marker の直後からを返す。marker が無ければ nil を返す。
func afterMarker(text []byte, marker string) []byte {
	index := bytes.Index(text, []byte(marker))
	if index < 0 {
		return nil
	}
	return text[index+len(marker):]
}

func isSVGStartTag(text []byte) bool {
	if !bytes.HasPrefix(text, []byte("<svg")) || len(text) == len("<svg") {
		return false
	}
	return bytes.IndexByte([]byte(" \t\r\n>/:"), text[len("<svg")]) >= 0
}
