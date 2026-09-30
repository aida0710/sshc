package terminal

import (
	"sync"
	"sync/atomic"
)

// ひとつのセッションの出力を、アタッチしているもの（ブラウザのターミナルの表示）へ
// 配る。追いつけないアタッチは落とし、PTY は止めない。

// streamDepth は、ひとつのアタッチが溜め込めるチャンクの数である。
const streamDepth = 256

// Stream は、ひとつのアタッチである。
type Stream struct {
	output  chan []byte
	closed  sync.Once
	dropped atomic.Bool
}

// Output は、このアタッチへ配る出力である。アタッチが外れると閉じる。
func (s *Stream) Output() <-chan []byte { return s.output }

// Dropped は、このアタッチが追いつけずに落とされたかを報告する。
func (s *Stream) Dropped() bool { return s.dropped.Load() }

func (s *Stream) close() { s.closed.Do(func() { close(s.output) }) }

// CanAttachFrom reports whether cursor belongs to the output range written by
// this session. An old cursor is valid and will be marked truncated; a cursor
// ahead of the writer is not.
func (s *Session) CanAttachFrom(cursor uint64) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buffer.CanReadFrom(cursor)
}

// AttachFrom atomically returns only output after cursor and then follows live
// output. Registering the stream under the same lock as the range read leaves
// no gap between replay and live delivery.
func (s *Session) AttachFrom(cursor uint64) (RingRead, *Stream, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	replay, ok := s.buffer.ReadAvailableFrom(cursor)
	if !ok {
		return RingRead{}, nil, false
	}
	stream := &Stream{output: make(chan []byte, streamDepth)}
	if s.exited != nil {
		// 終了済みのセッションにライブの出力は無い。読めるものを渡してから閉じる。
		stream.close()
		return replay, stream, true
	}
	s.streams[stream] = true
	return replay, stream, true
}

// Detach は接続を解除する。セッション自体は継続する。
func (s *Session) Detach(stream *Stream) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.streams[stream] {
		delete(s.streams, stream)
	}
	stream.close()
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
		stream.close()
	}
}

// publish は、出力をバッファへ書き、アタッチしているものへ配る。
func (s *Session) publish(chunk []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, _ = s.buffer.Write(chunk)
	for stream := range s.streams {
		// 複製するのは、この配列を次の読み取りが上書きするからである。
		copied := make([]byte, len(chunk))
		copy(copied, chunk)
		select {
		case stream.output <- copied:
		default:
			// 追いつけないアタッチは落とす。PTY は止めない。
			stream.dropped.Store(true)
			delete(s.streams, stream)
			stream.close()
		}
	}
}
