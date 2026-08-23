package terminal

// Ring は、セッションひとつ分のスクロールバックである。
type Ring struct {
	data  []byte
	start int
	size  int
}

func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		capacity = 1
	}
	return &Ring{data: make([]byte, capacity)}
}

func (r *Ring) Len() int { return r.size }

func (r *Ring) Write(p []byte) (int, error) {
	written := len(p)
	capacity := len(r.data)
	if written >= capacity {
		copy(r.data, p[written-capacity:])
		r.start, r.size = 0, capacity
		return written, nil
	}

	// 書き込みの開始位置は、いま保持している末尾の次である。
	end := (r.start + r.size) % capacity
	first := copy(r.data[end:], p)
	copy(r.data, p[first:])

	r.size += written
	if r.size > capacity {
		// あふれた分だけ、いちばん古いバイトが押し出される。
		r.start = (r.start + (r.size - capacity)) % capacity
		r.size = capacity
	}
	return written, nil
}

// Snapshot は、いま保持しているバイト列を古い順に返す。
func (r *Ring) Snapshot() []byte {
	out := make([]byte, r.size)
	if r.size == 0 {
		return out
	}
	first := copy(out, r.data[r.start:])
	copy(out[first:], r.data)
	return out
}
