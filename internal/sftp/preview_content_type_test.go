package sftp

import "testing"

func TestSVGIsRecognizedByItsRootElementAfterTheProlog(t *testing.T) {
	tests := []struct {
		name string
		head string
		want string
	}{
		{name: "bare root", head: `<svg xmlns="http://www.w3.org/2000/svg"/>`, want: "image/svg+xml"},
		{name: "byte order mark and declaration", head: "\xEF\xBB\xBF<?xml version=\"1.0\"?>\n<svg>", want: "image/svg+xml"},
		{name: "comment and doctype", head: "<!-- made by a tool -->\n<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\">\n<svg\n width=\"1\">", want: "image/svg+xml"},
		{name: "other xml root", head: `<?xml version="1.0"?><feed>`, want: ""},
		{name: "html", head: "<html><svg></svg></html>", want: ""},
		{name: "name that only begins with svg", head: "<svgx>", want: ""},
		{name: "comment cut off before the root", head: "<!-- a long comment", want: ""},
		{name: "plain text mentioning svg", head: "draw an <svg> here", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := previewContentType([]byte(test.head)); got != test.want {
				t.Fatalf("previewContentType(%q) = %q, want %q", test.head, got, test.want)
			}
		})
	}
}
