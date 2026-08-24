package httpserver

import "testing"

func TestDownloadOffsetAcceptsOnlySingleOpenEndedRange(t *testing.T) {
	tests := []struct {
		header string
		size   int64
		want   int64
		ranged bool
		valid  bool
	}{
		{header: "", size: 10, want: 0, valid: true},
		{header: "bytes=4-", size: 10, want: 4, ranged: true, valid: true},
		{header: "bytes=10-", size: 10},
		{header: "bytes=-4", size: 10},
		{header: "bytes=1-2", size: 10},
		{header: "bytes=1-,4-", size: 10},
	}
	for _, test := range tests {
		offset, ranged, err := downloadOffset(test.header, test.size)
		if (err == nil) != test.valid || offset != test.want || ranged != test.ranged {
			t.Errorf("downloadOffset(%q, %d) = %d, %v, %v", test.header, test.size, offset, ranged, err)
		}
	}
}
