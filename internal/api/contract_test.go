package api

import "encoding/json"

// このファイルは、生成された models.gen.go が、ハンドラと画面の頼るフィールドを
// 持ち続けていることをコンパイルで確かめる。OpenAPI の変更でフィールドの名前や型が
// 変わると、このファイルがコンパイルできなくなり、go test ./internal/api が失敗する。
// 代入した値を実行時に読み返す比較は必ず成り立つだけなので置かない。

// 基盤の応答。
var (
	_ = HealthResponse{Status: "ok", Version: "dev"}
	_ = BootstrapResponse{CsrfToken: "csrf"}
)

// 接続の編集。変えない項目を送らずに済むよう、HostName は省略できる。
var (
	_ = UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"},
		Base:     "Host edge\n",
		Password: json.RawMessage(`{"kind":"unchanged"}`),
	}
	_ *json.RawMessage = UpdateConnectionRequest{}.HostName
)

// 鍵の一覧と、鍵の操作の応答。証明書は無い鍵が普通なので省略できる。
var (
	_ = KeyItem{
		Id:             "0123456789abcdef0123456789abcdef",
		RelativePath:   "id_work",
		Kind:           "private_key",
		Container:      "OPENSSH PRIVATE KEY",
		Algorithm:      "ed25519",
		KeyType:        "ssh-ed25519",
		Bits:           256,
		Encrypted:      true,
		Fingerprint:    "SHA256:abcdef",
		Comment:        "aida@laptop",
		Permission:     "0600",
		PermissionRisk: false,
		SizeBytes:      444,
		References: []KeyReference{{
			Directive:    "IdentityFile",
			ConfigPath:   "/Users/example/.ssh/config",
			Line:         2,
			Condition:    "Host build-*",
			HostPatterns: []string{"build-*"},
			Value:        "~/.ssh/id_work",
		}},
		Notes: []string{},
	}
	_ *KeyCertificate = KeyItem{}.Certificate
	_                 = KeyCertificate{
		KeyId:                "probe-id",
		Principals:           []string{"alice"},
		ValidBefore:          0,
		NeverExpires:         true,
		SignedKeyType:        "ssh-ed25519",
		SignedKeyFingerprint: "SHA256:abcdef",
	}
	_ = KeyInventoryResponse{
		Items:                []KeyItem{},
		Unreadable:           []UnreadableFile{{RelativePath: "huge_known_hosts", Reason: "file_too_large"}},
		AgentDelegations:     []KeyReference{},
		UnresolvedReferences: []UnresolvedReference{},
		AgentAvailable:       true,
		AgentIdentities:      []AgentIdentity{{Bits: 256, Fingerprint: "SHA256:abcdef", Comment: "aida@laptop", Algorithm: "ED25519"}},
	}
	_ = RevealPrivateKeyResponse{
		Id:            "0123456789abcdef0123456789abcdef",
		RelativePath:  "id_work",
		PrivateKey:    "-----BEGIN OPENSSH PRIVATE KEY-----\n",
		Encrypted:     true,
		Fingerprint:   "SHA256:abcdef",
		TransactionId: "20260805T090000.000-aabbccdd",
	}
	_ = KeyAlgorithmsResponse{
		Variants: []KeyVariant{{Algorithm: "ed25519-sk", Bits: 0, Label: "Ed25519 security key", InProcess: false, Reason: "hardware_token_required"}},
		Source:   "ssh -Q key",
	}
	_ = GenerateKeyResponse{
		Id: "0123456789abcdef0123456789abcdef", RelativePath: "id_work", PublicRelativePath: "id_work.pub",
		Fingerprint: "SHA256:abcdef", KeyType: "ssh-ed25519", Bits: 256,
		Encrypted: true, TransactionId: "20260805T090000.000-aabbccdd",
	}
	_ = HardwareCommandResponse{
		Algorithm: "ed25519-sk",
		Command:   []string{"ssh-keygen", "-t", "ed25519-sk"},
		Note:      "run this in Terminal",
	}
	_ = ChangePassphraseResponse{
		Id: "0123456789abcdef0123456789abcdef", RelativePath: "id_work", Encrypted: true,
		Notes: []string{}, TransactionId: "20260805T090000.000-aabbccdd",
	}
	_ = RegisterKeyResponse{
		Id: "0123456789abcdef0123456789abcdef", RelativePath: "id_work", Fingerprint: "SHA256:abcdef",
		LifetimeSeconds: 3600, Identities: []AgentIdentity{},
	}
)

// 確認付きの操作。リクエストはコミット済みのセッション用語である kind と target を
// 指定する。evidence は呼び出し側からは決して渡されない。サーバーが、確認ダイアログに
// 表示されていた内容から導出する。
var (
	_ = IssueActionRequest{Kind: "private_key.reveal", Target: "0123456789abcdef0123456789abcdef"}
	_ = IssueActionResponse{Token: "t", ExpiresAt: "2026-08-05T09:02:00Z"}
)

// ゴミ箱。
var (
	_ = TrashListResponse{
		Entries: []TrashEntrySummary{{
			Id:         "20260805T090000.000-aabbccdd",
			DeletedAt:  "2026-08-05T09:00:00Z",
			AgeDays:    40,
			Stale:      true,
			Files:      []TrashFileSummary{{OriginalRelativePath: "id_work", TrashRelativePath: "sshc/trash/e/id_work", Kind: "private_key", Fingerprint: "SHA256:abcdef", Permission: "0600"}},
			Restorable: false,
			Blockers:   []string{"restore_path_occupied:id_work"},
		}},
		RetentionDays: 30,
	}
	_ = TrashKeyResponse{EntryId: "e", Files: []TrashFileSummary{}, Skipped: []string{}, TransactionId: "t"}
	_ = RestoreTrashResponse{EntryId: "e", Restored: []string{"id_work"}, Blockers: []string{}, TransactionId: "t"}
	_ = PurgeTrashResponse{EntryId: "e", Removed: []string{"id_work"}, TransactionId: "t"}
)

// 失敗の応答。detail は無いこともあるので省略できる。
var (
	_         = Problem{Code: "agent_rejected", Message: "request rejected"}
	_ *string = Problem{}.Detail
)
