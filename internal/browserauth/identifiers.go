package browserauth

import "strconv"

// registrationIdentifiers は、登録ごとにこのプロセスの中だけで使う識別子を持つ。
//
// 登録から入ったセッションを、その登録が消えたときにまとめて失効させるために使う。
// 登録の文書は識別子を持たず、token のハッシュは rotation のたびに変わるので、
// 現在のハッシュから引き、rotation で付け替える。結び付けるセッションは engine の
// 再起動で消えるので、識別子もメモリにあれば足りる。Store の mutex の中で使う。
type registrationIdentifiers struct {
	byHash map[string]string
	issued uint64
}

// of は、現在のハッシュが hash の登録の識別子を返す。初めて見た登録には付ける。
func (identifiers *registrationIdentifiers) of(hash string) string {
	if identifier, ok := identifiers.byHash[hash]; ok {
		return identifier
	}
	identifiers.issued++
	identifier := "registration-" + strconv.FormatUint(identifiers.issued, 10)
	identifiers.byHash[hash] = identifier
	return identifier
}

// rotate は、token を差し替えた登録に同じ識別子を付け直し、それを返す。
func (identifiers *registrationIdentifiers) rotate(previousHash, currentHash string) string {
	identifier := identifiers.of(previousHash)
	delete(identifiers.byHash, previousHash)
	identifiers.byHash[currentHash] = identifier
	return identifier
}

// keepOnly は、ディスクに残っている登録の識別子だけを残す。
func (identifiers *registrationIdentifiers) keepOnly(registrations []registration) {
	for hash := range identifiers.byHash {
		if indexOf(registrations, hash, currentHash) < 0 {
			delete(identifiers.byHash, hash)
		}
	}
}
