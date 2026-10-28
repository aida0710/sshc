package terminal

// ひとつのセッションの出力を、アタッチしているもの（ブラウザのターミナルの表示）へ
// 配る。PTY は止めない。追いつけないアタッチは、再生バッファに残っているところまで
// 先へ飛ぶ。

// Stream は、ひとつのアタッチである。
//
// 出力を写して溜めるのではなく、セッションの再生バッファの読み位置（cursor）を持つ。
// 受け手は取り出せるだけまとめて取り出すので、PTY の読み取りの大きさ（macOS の
// PTY は1回に約 1 KiB しか返さない）に左右されない。受け手が遅れて、読み位置が
// 再生バッファから押し出されたら、残っている最初のところから続ける。繋ぎ直しても
// それより前は取り戻せないので、アタッチは外さない。
type Stream struct {
	session *Session
	// cursor と open は session.mutex で守る。
	cursor uint64
	open   bool
	// ready は、取り出せる出力が届いたか、アタッチが外れたことを知らせる。
	ready chan struct{}
	// shellReplaced は、表示していたシェルが新しいシェルに替わったことを知らせる。
	// 受け手が知りたいのは替わったかどうかだけなので、知らせは 1 つ溜まれば足りる。
	shellReplaced chan struct{}
}

func newStream(session *Session, cursor uint64) *Stream {
	return &Stream{
		session: session, cursor: cursor, open: true,
		ready: make(chan struct{}, 1), shellReplaced: make(chan struct{}, 1),
	}
}

// Ready は、Take で取り出せる出力が届いたか、アタッチが外れたことを知らせる。
func (s *Stream) Ready() <-chan struct{} { return s.ready }

// StreamRead は、Take が取り出した出力である。
type StreamRead struct {
	Data []byte
	// Start は Data の最初のバイトの位置である。
	Start uint64
	// Skipped は、前に取り出した出力と Start のあいだが、受け手が遅れているうちに
	// 再生バッファから押し出されたことを表す。Data は空のこともある。
	Skipped bool
}

// Take は、届いている出力を limit バイトまで取り出す。open は、アタッチが外れて
// 残りも取り出し終えたら false になる。
func (s *Stream) Take(limit int) (read StreamRead, open bool) {
	session := s.session
	session.mutex.Lock()
	defer session.mutex.Unlock()
	output, _ := session.readOutputLocked(s.cursor, limit)
	s.cursor = output.Next
	// 取り出し残しと、外れたことは、次の Take で受け取らせる。
	if output.Next < output.End || !s.open {
		s.signal()
	}
	read = StreamRead{Data: output.Data, Start: output.Start, Skipped: output.Truncated}
	return read, s.open || len(read.Data) > 0 || read.Skipped
}

func (s *Stream) signal() {
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// ShellReplaced は、再接続でシェルが替わるたびに知らせる。ブラウザが前のシェルへ
// 送るつもりで確認していた貼り付けを取り消すのに使う。
func (s *Stream) ShellReplaced() <-chan struct{} { return s.shellReplaced }

func (s *Stream) announceShellReplaced() {
	select {
	case s.shellReplaced <- struct{}{}:
	default:
		// 前の知らせがまだ読まれていない。それが既に「替わった」と伝えている。
	}
}

// closeLocked はアタッチを外す。届いている出力は、受け手が取り出し終えるまで
// 取り出せる。session.mutex を持って呼ぶ。
func (s *Stream) closeLocked() {
	if !s.open {
		return
	}
	s.open = false
	s.signal()
}

// CanAttachFrom reports whether cursor belongs to the output range written by
// this session. An old cursor is valid and will be marked truncated; a cursor
// ahead of the writer is not.
func (s *Session) CanAttachFrom(cursor uint64) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buffer.CanReadFrom(cursor)
}

// AttachFrom atomically returns only output after cursor and then follows live
// output. The stream starts where the replay ends, under the same lock as the
// range read, so nothing falls between replay and live delivery.
func (s *Session) AttachFrom(cursor uint64) (RingRead, *Stream, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	replay, ok := s.readOutputLocked(cursor, s.buffer.Capacity())
	if !ok {
		return RingRead{}, nil, false
	}
	stream := newStream(s, replay.Next)
	if s.exited != nil {
		// 終了済みのセッションにライブの出力は無い。読めるものを渡してから閉じる。
		stream.closeLocked()
		return replay, stream, true
	}
	s.streams[stream] = true
	return replay, stream, true
}

// readOutputLocked は、cursor 以後の出力を limit バイトまで読む。cursor が再生
// バッファより古いときは、残っているうちで制御文字列の途中でない最初の位置から
// 読み、Truncated を立てる。ブラウザが、バッファが半分に切った制御文字列の
// 後ろ半分を文字として表示しないためである。s.mutex を持って呼ぶ。
func (s *Session) readOutputLocked(cursor uint64, limit int) (RingRead, bool) {
	if !s.buffer.CanReadFrom(cursor) {
		return RingRead{}, false
	}
	if cursor >= s.buffer.Oldest() {
		return s.buffer.ReadAvailableFrom(cursor, limit)
	}
	read, ok := s.buffer.ReadAvailableFrom(s.controlStrings.SafeStart(s.buffer.Oldest()), limit)
	read.Truncated = true
	return read, ok
}

// Detach は接続を解除する。セッション自体は継続する。
func (s *Session) Detach(stream *Stream) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.streams, stream)
	stream.closeLocked()
}

// closeStreams は、アタッチしているものをすべて外す。
func (s *Session) closeStreams() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.closeStreamsLocked()
}

// closeStreamsLocked は closeStreams と同じことを、mutex を持った呼び出し側から行う。
func (s *Session) closeStreamsLocked() {
	for stream := range s.streams {
		delete(s.streams, stream)
		stream.closeLocked()
	}
}

// announceShellReplacedLocked は、アタッチしているものへシェルが替わったことを知らせる。
func (s *Session) announceShellReplacedLocked() {
	for stream := range s.streams {
		stream.announceShellReplaced()
	}
}

// publish は、出力をバッファへ書き、アタッチしているものへ届いたと知らせる。
func (s *Session) publish(chunk []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, _ = s.buffer.Write(chunk)
	s.controlStrings.Write(chunk)
	s.controlStrings.Forget(s.buffer.Oldest())
	for stream := range s.streams {
		stream.signal()
	}
}
