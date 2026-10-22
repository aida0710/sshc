package sftp

import (
	"encoding/binary"
	"sync"
)

// maxDirectoryEntries は、1つのフォルダの一覧で受け取る名前の上限。pkg/sftp の ReadDir は
// サーバーが EOF を返すまで名前をメモリに溜め続けるので、ここで止めないと、一覧を終わらせない
// サーバーが sshc エンジンのメモリを使い果たせる。木をたどる操作の上限（2万項目）より十分大きく
// して、1つのフォルダに多くのファイルを置く使い方を断らないようにする。この件数なら一覧は数十 MiB に収まる。
const maxDirectoryEntries = 100_000

const (
	sftpClosePacket   = 4
	sftpReaddirPacket = 12
	sftpStatusPacket  = 101
	// directoryEntryLimitStatus は、上限を超えた READDIR の応答の代わりに pkg/sftp へ渡す
	// status code。SFTP v3 の code（0〜8）と重ならないので、Client.ReadDir がサーバーの
	// 失敗と区別できる。サーバーが同じ code を送っても、一覧が上限の失敗になるだけで害は無い。
	directoryEntryLimitStatus = 0x7fff_0001
	// sftpFrameLengthBytes は、送る要求の先頭にある長さの欄。
	sftpFrameLengthBytes = 4
	// 応答の先頭は、種類の1バイトと要求ID。NAME はそのあとに名前の数が続く。
	sftpResponseHeaderBytes = 1 + 4
	sftpNamesCountEnd       = sftpResponseHeaderBytes + 4
)

// directoryEntryLimit は、ハンドルごとに READDIR で受け取った名前の数を数える。pkg/sftp は
// 一覧を少しずつ読む API を公開していないので、要求と応答の packet を見て数える。要求は
// 呼び出し側の goroutine、応答は pkg/sftp の受信の goroutine から届くので、mutex で守る。
type directoryEntryLimit struct {
	mu               sync.Mutex
	handlesByRequest map[uint32]string
	entriesByHandle  map[string]uint64
}

func newDirectoryEntryLimit() *directoryEntryLimit {
	return &directoryEntryLimit{handlesByRequest: make(map[uint32]string), entriesByHandle: make(map[string]uint64)}
}

// observeRequest は、送る要求のうち READDIR の要求IDとハンドルを覚え、CLOSE でそのハンドルの
// 件数を捨てる。frame は長さの欄から始まる要求の全体で、pkg/sftp が組み立てたものなので、
// 形が合わないものは数えずに通す。
func (limit *directoryEntryLimit) observeRequest(frame []byte) {
	if limit == nil || len(frame) < sftpFrameHeaderBytes {
		return
	}
	packetType := frame[sftpFrameLengthBytes]
	if packetType != sftpReaddirPacket && packetType != sftpClosePacket {
		return
	}
	requestID, handle, ok := requestIDAndHandle(frame[sftpFrameHeaderBytes:])
	if !ok {
		return
	}
	limit.mu.Lock()
	defer limit.mu.Unlock()
	if packetType == sftpReaddirPacket {
		limit.handlesByRequest[requestID] = handle
		return
	}
	delete(limit.entriesByHandle, handle)
}

// overLimitStatus は、READDIR への NAME の応答で名前を数え、ハンドルの合計が上限を超えたら、
// その応答の代わりに渡す directoryEntryLimitStatus の STATUS を返す。超えていなければ nil。
// packet は長さの欄を除いた応答。
func (limit *directoryEntryLimit) overLimitStatus(packet []byte) []byte {
	if limit == nil || len(packet) < sftpResponseHeaderBytes {
		return nil
	}
	packetType := packet[0]
	if packetType != sftpNamesPacket && packetType != sftpStatusPacket {
		return nil
	}
	requestID := binary.BigEndian.Uint32(packet[1:sftpResponseHeaderBytes])
	limit.mu.Lock()
	defer limit.mu.Unlock()
	handle, isReaddir := limit.handlesByRequest[requestID]
	if !isReaddir {
		return nil
	}
	delete(limit.handlesByRequest, requestID)
	if packetType != sftpNamesPacket || len(packet) < sftpNamesCountEnd {
		return nil
	}
	limit.entriesByHandle[handle] += uint64(binary.BigEndian.Uint32(packet[sftpResponseHeaderBytes:sftpNamesCountEnd]))
	if limit.entriesByHandle[handle] <= maxDirectoryEntries {
		return nil
	}
	return directoryEntryLimitStatusPacket(requestID)
}

// requestIDAndHandle は、要求IDに続いて長さ付きのハンドルを持つ要求（READDIR、CLOSE）を読む。
func requestIDAndHandle(body []byte) (uint32, string, bool) {
	const handleOffset = 8 // 要求IDと、ハンドルの長さの欄。
	if len(body) < handleOffset {
		return 0, "", false
	}
	handleBytes := uint64(binary.BigEndian.Uint32(body[4:handleOffset]))
	if handleBytes > uint64(len(body)-handleOffset) {
		return 0, "", false
	}
	return binary.BigEndian.Uint32(body[:4]), string(body[handleOffset : handleOffset+int(handleBytes)]), true
}

// directoryEntryLimitStatusPacket は、空のメッセージと言語タグを持つ STATUS の応答を作る。
func directoryEntryLimitStatusPacket(requestID uint32) []byte {
	packet := make([]byte, 0, sftpResponseHeaderBytes+3*4)
	packet = append(packet, sftpStatusPacket)
	packet = binary.BigEndian.AppendUint32(packet, requestID)
	packet = binary.BigEndian.AppendUint32(packet, directoryEntryLimitStatus)
	packet = binary.BigEndian.AppendUint32(packet, 0) // メッセージ
	return binary.BigEndian.AppendUint32(packet, 0)   // 言語タグ
}
