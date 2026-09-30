package keys

import (
	"context"
	"errors"

	"sshc/internal/platform"
	"sshc/internal/storage"
)

// ErrNoPublicKey は、隣に公開鍵の片割れがない秘密鍵を報告する。エージェントからの
// 取り消しは公開鍵で鍵を指定するので、公開鍵が無いと取り消せない。
var ErrNoPublicKey = errors.New("this key has no public half, which is what removing it from the agent needs")

// RegisterRequest は、鍵をひとつ読み込むようユーザーの ssh-agent に求める。
type RegisterRequest struct {
	KeyID           string
	Passphrase      []byte
	LifetimeSeconds int
}

type RegisterResult struct {
	ID              string
	RelativePath    string
	Fingerprint     string
	LifetimeSeconds int
	Identities      []platform.AgentIdentity
}

// Register は秘密鍵をユーザーの ssh-agent へ読み込ませる。
//
// 登録できるのは、いまインベントリに含まれる鍵だけである。したがって、ごみ箱に
// ある鍵と ~/.ssh/sshc 配下のものは、構造上到達できない。パスフレーズは Register
// が返る前に上書きされ、登録はそれを含まずに履歴へ記録される。監査の記録が書かれる
// のはエージェントが鍵を受け付けたあとだけなので、拒否された登録が、それが起きたと
// 主張する記録を残すことは
// ない。
func (service *Service) Register(ctx context.Context, request RegisterRequest) (RegisterResult, error) {
	defer clear(request.Passphrase)

	if service.agent == nil {
		return RegisterResult{}, platform.ErrAgentUnavailable
	}
	_, item, err := service.privateKey(request.KeyID)
	if err != nil {
		return RegisterResult{}, err
	}

	// 呼び出し側が何も渡さなかったときには、保存されているパスフレーズを使う。それが、
	// エージェントへの鍵の追加を二段階ではなく一度の操作にしている。ただし、打ち込まれた
	// ものより優先されることは決してない。キーボードの前にいるユーザーの方が、ファイルよりも
	// 新しいからである。
	passphrase := request.Passphrase
	if len(passphrase) == 0 && service.storedPassphrase != nil {
		if stored, ok := service.storedPassphrase(item.RelativePath); ok {
			passphrase = []byte(stored)
			defer clear(passphrase)
		}
	}

	contents, err := service.readScanned(item)
	if err != nil {
		return RegisterResult{}, err
	}
	defer clear(contents)
	absolute := service.absolutePath(item.RelativePath)
	if err := service.agent.Add(ctx, platform.AgentAddRequest{
		PrivateKey:      contents,
		Comment:         absolute,
		Passphrase:      passphrase,
		LifetimeSeconds: request.LifetimeSeconds,
	}); err != nil {
		return RegisterResult{}, err
	}
	if _, err := service.transactions.Note("key.agent_add", []string{absolute}); err != nil {
		return RegisterResult{}, err
	}

	identities, listErr := service.agent.List(ctx)
	if listErr != nil {
		identities = nil
	}
	return RegisterResult{
		ID:              item.ID,
		RelativePath:    item.RelativePath,
		Fingerprint:     item.Fingerprint,
		LifetimeSeconds: request.LifetimeSeconds,
		Identities:      identities,
	}, nil
}

// Deregister は、鍵ひとつをエージェントから外す。
//
// これが無いと、鍵を完全削除しても、利用者が破棄したばかりの鍵をエージェントが
// 持ち続け、エージェントの保持内容を並べる画面はそれを並べ続ける。
//
// agent プロトコルの削除要求は、鍵を公開鍵の blob で指定する。その blob は公開鍵
// ファイルから読むので、公開鍵の片割れが要る。見つからないときは、行われていない
// 削除を主張せず、エージェントには触れずに ErrNoPublicKey を返す。
func (service *Service) Deregister(ctx context.Context, keyID string) error {
	if service.agent == nil {
		return platform.ErrAgentUnavailable
	}
	inventory, item, err := service.privateKey(keyID)
	if err != nil {
		return err
	}
	public, ok := publicKeyFor(inventory, item)
	if !ok {
		return ErrNoPublicKey
	}
	publicKey, err := service.readScanned(public)
	if err != nil {
		return err
	}
	if err := service.agent.Remove(ctx, publicKey); err != nil {
		return err
	}
	_, err = service.transactions.Note("key.agent_remove", []string{service.absolutePath(item.RelativePath)})
	return err
}

// readScanned は、いま走査したインベントリの項目の中身を読む。
//
// 読み方はほかの鍵の読み込みと同じく、シンボリックリンクをたどらず、大きさに上限を
// 設ける。走査のあとで中身が変わっていれば ErrKeyChanged を返し、走査で確かめた
// ものと違うファイルを agent へ渡さない。
func (service *Service) readScanned(item *Item) ([]byte, error) {
	contents, err := service.workspace.FileSystem().ReadFile(service.absolutePath(item.RelativePath))
	if err != nil {
		return nil, err
	}
	if storage.Digest(contents) != item.ContentDigest {
		clear(contents)
		return nil, ErrKeyChanged
	}
	return contents, nil
}

// publicKeyFor は、秘密鍵の公開鍵の片割れをフィンガープリントで探し、見つから
// なければ、隣にある慣例的な ".pub" という名前へフォールバックする。
func publicKeyFor(inventory *Inventory, item *Item) (*Item, bool) {
	for index := range inventory.Items {
		candidate := &inventory.Items[index]
		if candidate.Kind != KindPublicKey {
			continue
		}
		if item.Fingerprint != "" && candidate.Fingerprint == item.Fingerprint {
			return candidate, true
		}
		if candidate.RelativePath == item.RelativePath+".pub" {
			return candidate, true
		}
	}
	return nil, false
}

// AgentIdentities は、エージェントがいま保持しているものを報告する。二つ目の
// 戻り値は、到達できるエージェントがないときに false になる。UI が、動いている
// エージェントに見える空リストではなく、その旨を言えるようにするためだ。
func (service *Service) AgentIdentities(ctx context.Context) ([]platform.AgentIdentity, bool) {
	if service.agent == nil || !service.agent.Available(ctx) {
		return nil, false
	}
	identities, err := service.agent.List(ctx)
	if err != nil {
		return nil, false
	}
	return identities, true
}
