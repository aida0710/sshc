package sftp

import (
	"path"
	"strings"
)

// mediaContentTypes は、プレビューで再生する音声・動画の拡張子と型である。
// 画像と違い、型は名前から決める。<audio>・<video> は宣言された型で script を
// 動かすことが無く、再生できない中身はブラウザが再生しないだけだからである。
var mediaContentTypes = map[string]string{
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".opus": "audio/ogg",
	".flac": "audio/flac",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".mkv":  "video/x-matroska",
	".ogv":  "video/ogg",
}

// MediaContentType は、名前が再生できる音声・動画なら型を返し、そうでなければ
// 空を返す。
func MediaContentType(name string) string {
	return mediaContentTypes[strings.ToLower(path.Ext(name))]
}
