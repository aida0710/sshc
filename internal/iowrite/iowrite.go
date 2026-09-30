// Package iowrite は、io.Writer へ書くときの共通の規則を置く。残らず書き切る
// WriteAll と、上限まで覚えて残りを捨てる CappedBuffer である。
package iowrite

import (
	"io"
	"slices"
	"sync"
)

// WriteAll は payload を残らず書く。
//
// 短い書き込みは進捗として続きを書く。0バイトの書き込みは、それ以上進まないので
// io.ErrShortWrite にする。書き手が payload より長い、または負の長さを返したときも、
// どこまで書けたか分からないので io.ErrShortWrite にする。
func WriteAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		count, err := writer.Write(payload)
		if count < 0 || count > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[count:]
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// CappedBuffer は、上限まで書き込みを覚え、それ以降は捨てる書き込み先である。
//
// 書き手にはエラーを返さず、捨てた分も書けたと返す。エラーを返すと、上限に
// 達したことがコマンドの失敗として伝わり、os/exec はそこで写しを止めてプログラム側の
// 書き込みが詰まる。捨てたかどうかは Truncated が返す。並行に書いても読んでもよい。
type CappedBuffer struct {
	limit     int
	mutex     sync.Mutex
	kept      []byte
	truncated bool
}

// NewCappedBuffer は、limit バイトまで覚える CappedBuffer を返す。
func NewCappedBuffer(limit int) *CappedBuffer {
	return &CappedBuffer{limit: limit}
}

func (b *CappedBuffer) Write(chunk []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	kept := chunk
	if room := max(b.limit-len(b.kept), 0); len(kept) > room {
		kept = kept[:room]
		b.truncated = true
	}
	b.kept = append(b.kept, kept...)
	return len(chunk), nil
}

// Bytes は、覚えている内容の写しを返す。
func (b *CappedBuffer) Bytes() []byte {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return slices.Clone(b.kept)
}

// String は、覚えている内容を文字列で返す。
func (b *CappedBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return string(b.kept)
}

// Truncated は、上限を超えて捨てた書き込みがあったかを返す。
func (b *CappedBuffer) Truncated() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.truncated
}
