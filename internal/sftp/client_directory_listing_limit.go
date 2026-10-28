package sftp

import (
	"encoding/binary"
	"sync"
)

// maxDirectoryEntries は、1つのフォルダの一覧で受け取る名前の上限。pkg/sftp の ReadDir は
// サーバーが EOF を返すまで名前をメモリに溜め続けるので、ここで止めないと、一覧を終わらせない
// サーバーが sshc エンジンのメモリを使い果たせる。木をたどる操作の上限（2万項目）より十分大きく
// して、1つのフォルダに多くのファイルを置く使い方を断らないようにする。
const maxDirectoryEntries = 100_000

// maxDirectoryListingBytes は、1つのフォルダの一覧で受け取る NAME の応答のバイト数の合計の上限。
// 名前・longname・属性はすべてこの応答に入り、pkg/sftp は名前と拡張属性を一覧の終わりまで溜めるので、
// 件数だけを数えると、長い名前を返し続けるサーバーが件数の上限より前にメモリを使い果たせる。
// 10万項目のすべてが 255 バイトの名前とそれを含む longname を持っても（1項目およそ 600 バイト）収まる大きさにする。
const maxDirectoryListingBytes = 64 << 20

const (
	sftpClosePacket   = 4
	sftpReaddirPacket = 12
	sftpStatusPacket  = 101
	// directoryEntryLimitStatus と directoryBytesLimitStatus は、上限を超えた READDIR の応答の
	// 代わりに pkg/sftp へ渡す status code。SFTP v3 の code（0〜8）と重ならないので、
	// Client.ReadDir がサーバーの失敗と区別できる。サーバーが同じ code を送っても、一覧が上限の
	// 失敗になるだけで害は無い。
	directoryEntryLimitStatus = 0x7fff_0001
	directoryBytesLimitStatus = 0x7fff_0002
	// sftpFrameLengthBytes は、送る要求の先頭にある長さの欄。
	sftpFrameLengthBytes = 4
	// 応答の先頭は、種類の1バイトと要求ID。NAME はそのあとに名前の数が続く。
	sftpResponseHeaderBytes = 1 + 4
	sftpNamesCountEnd       = sftpResponseHeaderBytes + 4
)

// directoryListingLimit は、ハンドルごとに READDIR で受け取った名前の数と応答のバイト数を数える。
// pkg/sftp は一覧を少しずつ読む API を公開していないので、要求と応答の packet を見て数える。要求は
// 呼び出し側の goroutine、応答は pkg/sftp の受信の goroutine から届くので、mutex で守る。
type directoryListingLimit struct {
	mutex            sync.Mutex
	handlesByRequest map[uint32]string
	totalsByHandle   map[string]directoryListingTotals
}

// directoryListingTotals は、1つのハンドルの一覧でここまでに受け取った量。
type directoryListingTotals struct {
	entries uint64
	bytes   uint64
}

func newDirectoryListingLimit() *directoryListingLimit {
	return &directoryListingLimit{handlesByRequest: make(map[uint32]string), totalsByHandle: make(map[string]directoryListingTotals)}
}

// observeRequest は、送る要求のうち READDIR の要求IDとハンドルを覚え、CLOSE でそのハンドルの
// 合計を捨てる。frame は長さの欄から始まる要求の全体で、pkg/sftp が組み立てたものなので、
// 形が合わないものは数えずに通す。
func (limit *directoryListingLimit) observeRequest(frame []byte) {
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
	limit.mutex.Lock()
	defer limit.mutex.Unlock()
	if packetType == sftpReaddirPacket {
		limit.handlesByRequest[requestID] = handle
		return
	}
	delete(limit.totalsByHandle, handle)
}

// overLimitStatus は、READDIR への NAME の応答で名前の数とバイト数を足し、ハンドルの合計が
// どちらかの上限を超えたら、その応答の代わりに渡す STATUS を返す。超えていなければ nil。
// packet は長さの欄を除いた応答。
func (limit *directoryListingLimit) overLimitStatus(packet []byte) []byte {
	if limit == nil || len(packet) < sftpResponseHeaderBytes {
		return nil
	}
	packetType := packet[0]
	if packetType != sftpNamesPacket && packetType != sftpStatusPacket {
		return nil
	}
	requestID := binary.BigEndian.Uint32(packet[1:sftpResponseHeaderBytes])
	limit.mutex.Lock()
	defer limit.mutex.Unlock()
	handle, isReaddir := limit.handlesByRequest[requestID]
	if !isReaddir {
		return nil
	}
	delete(limit.handlesByRequest, requestID)
	if packetType != sftpNamesPacket || len(packet) < sftpNamesCountEnd {
		return nil
	}
	totals := limit.totalsByHandle[handle]
	totals.entries += uint64(binary.BigEndian.Uint32(packet[sftpResponseHeaderBytes:sftpNamesCountEnd]))
	totals.bytes += uint64(len(packet))
	limit.totalsByHandle[handle] = totals
	switch {
	case totals.entries > maxDirectoryEntries:
		return directoryListingLimitStatusPacket(requestID, directoryEntryLimitStatus)
	case totals.bytes > maxDirectoryListingBytes:
		return directoryListingLimitStatusPacket(requestID, directoryBytesLimitStatus)
	default:
		return nil
	}
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

// directoryListingLimitStatusPacket は、空のメッセージと言語タグを持つ STATUS の応答を作る。
func directoryListingLimitStatusPacket(requestID, statusCode uint32) []byte {
	packet := make([]byte, 0, sftpResponseHeaderBytes+3*4)
	packet = append(packet, sftpStatusPacket)
	packet = binary.BigEndian.AppendUint32(packet, requestID)
	packet = binary.BigEndian.AppendUint32(packet, statusCode)
	packet = binary.BigEndian.AppendUint32(packet, 0) // メッセージ
	return binary.BigEndian.AppendUint32(packet, 0)   // 言語タグ
}
