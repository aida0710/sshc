package sftp

import (
	"errors"
	"io"
)

// rangeReadSeeker は、接続先のファイルを、求められた位置から読み直せる
// io.ReadSeeker にする。再生中に飛んだ先だけを読み、手前を読み捨てない。
// 読み出しは最初の Read まで開かない。http.ServeContent は大きさを知るために
// 末尾へ Seek するが、そこから読むことはない。
type rangeReadSeeker struct {
	remote RangeRemote
	path   string
	size   int64
	offset int64
	reader io.ReadCloser
}

func (r *rangeReadSeeker) Read(buffer []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}
	if r.reader == nil {
		reader, err := r.remote.OpenRange(r.path, r.offset)
		if err != nil {
			return 0, err
		}
		r.reader = reader
	}
	read, err := r.reader.Read(buffer)
	r.offset += int64(read)
	return read, err
}

func (r *rangeReadSeeker) Seek(offset int64, whence int) (int64, error) {
	target := offset
	switch whence {
	case io.SeekCurrent:
		target += r.offset
	case io.SeekEnd:
		target += r.size
	}
	if target < 0 {
		return r.offset, errors.New("sftp: seek before the start of the file")
	}
	if target != r.offset {
		r.closeReader()
		r.offset = target
	}
	return target, nil
}

func (r *rangeReadSeeker) closeReader() {
	if r.reader != nil {
		_ = r.reader.Close()
		r.reader = nil
	}
}
