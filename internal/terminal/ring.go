package terminal

// Ring は、セッションひとつ分のスクロールバックである。
type Ring struct {
	data  []byte
	start int
	size  int
	// written is the monotonic byte position immediately after the newest byte.
	// It deliberately does not wrap with the storage so an observer can ask for
	// only the bytes written since its previous read.
	written uint64
}

// RingRead is a bounded range from the retained scrollback. Start and Next are
// absolute byte positions. Truncated means the requested cursor had already
// fallen behind the oldest retained byte.
type RingRead struct {
	Data []byte
	// Context contains retained bytes from the oldest byte through Next. Emit
	// is the offset where Data begins. A terminal decoder can therefore carry
	// escape-sequence state across a cursor which lands inside CSI or OSC.
	Context   []byte
	Emit      int
	Start     uint64
	Next      uint64
	End       uint64
	Truncated bool
}

func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		capacity = 1
	}
	return &Ring{data: make([]byte, capacity)}
}

func (r *Ring) Len() int { return r.size }

func (r *Ring) CanReadFrom(cursor uint64) bool { return cursor <= r.written }

func (r *Ring) Write(p []byte) (int, error) {
	written := len(p)
	r.written += uint64(written)
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

// ReadFrom returns at most limit bytes beginning at cursor. A cursor newer than
// the current end is rejected so a cursor from another session or engine cannot
// silently skip future output.
func (r *Ring) ReadFrom(cursor uint64, limit int) (RingRead, bool) {
	return r.readFrom(cursor, limit, true)
}

// ReadAvailableFrom returns every retained byte at or after cursor without
// allocating the decoder context needed by the plain-text control API.
func (r *Ring) ReadAvailableFrom(cursor uint64) (RingRead, bool) {
	return r.readFrom(cursor, r.size, false)
}

func (r *Ring) readFrom(cursor uint64, limit int, includeContext bool) (RingRead, bool) {
	oldest := r.written - uint64(r.size)
	if !r.CanReadFrom(cursor) {
		return RingRead{}, false
	}
	start := cursor
	truncated := false
	if start < oldest {
		start = oldest
		truncated = true
	}
	available := r.written - start
	if limit < 0 {
		limit = 0
	}
	if uint64(limit) < available {
		available = uint64(limit)
	}
	data := make([]byte, int(available))
	if len(data) != 0 {
		offset := int(start - oldest)
		physical := (r.start + offset) % len(r.data)
		first := copy(data, r.data[physical:])
		copy(data[first:], r.data)
	}
	next := start + available
	var context []byte
	if includeContext {
		contextLength := int(next - oldest)
		context = make([]byte, contextLength)
		if contextLength != 0 {
			first := copy(context, r.data[r.start:])
			copy(context[first:], r.data)
		}
	}
	return RingRead{
		Data: data, Context: context, Emit: int(start - oldest),
		Start: start, Next: next, End: r.written, Truncated: truncated,
	}, true
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
