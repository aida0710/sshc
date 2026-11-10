package terminal

// Ring は、セッションひとつ分のスクロールバックである。
type Ring struct {
	data  []byte
	start int
	size  int
	// written は、いちばん新しいバイトのすぐ後ろの、増えるだけのバイトの位置である。見張る側が、前に読んだあとに
	// 書かれたバイトだけを求められるよう、格納場所と一緒に折り返さないようにしてある。
	written uint64
}

// RingRead は、残っているスクロールバックの、長さに上限のある範囲である。Start と Next はバイトの絶対位置で
// ある。Truncated は、求められたカーソルが、残っているいちばん古いバイトよりすでに後ろにあったことを表す。
type RingRead struct {
	Data []byte
	// Context は、いちばん古いバイトから Next までの、残っているバイトを持つ。Emit は、Data が始まる位置である。
	// そのため、カーソルが CSI や OSC の途中に落ちても、ターミナルのデコーダーはエスケープシーケンスの状態を
	// 引き継げる。
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

func (r *Ring) CanReadFrom(cursor uint64) bool { return cursor <= r.written }

// Capacity は、保持できるバイト数である。
func (r *Ring) Capacity() int { return len(r.data) }

// Oldest は、残っているいちばん古いバイトの絶対位置である。
func (r *Ring) Oldest() uint64 { return r.written - uint64(r.size) }

// Written は、いちばん新しいバイトのすぐ後ろの絶対位置である。
func (r *Ring) Written() uint64 { return r.written }

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

// ReadFrom は、cursor から始まる、多くても limit バイトを返す。今の終わりより新しい cursor は断る。ほかの
// セッションやエンジンの cursor が、これから出る出力を黙って飛ばさないようにするためである。
func (r *Ring) ReadFrom(cursor uint64, limit int) (RingRead, bool) {
	return r.readFrom(cursor, limit, true)
}

// ReadAvailableFrom は、cursor 以降の、残っているバイトを、多くても limit だけ返す。平文の制御 API が要る
// デコーダーの文脈は確保しない。
func (r *Ring) ReadAvailableFrom(cursor uint64, limit int) (RingRead, bool) {
	return r.readFrom(cursor, limit, false)
}

func (r *Ring) readFrom(cursor uint64, limit int, includeContext bool) (RingRead, bool) {
	oldest := r.Oldest()
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
