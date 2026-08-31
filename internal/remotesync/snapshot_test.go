package remotesync_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sshc/internal/remotesync"
	"sshc/internal/storage"
)

func entry(path, contents string) remotesync.Entry {
	return remotesync.Entry{
		Path:   path,
		SHA256: remotesync.Digest([]byte(contents)),
		Mode:   "0600",
	}
}

func buildFixture(t *testing.T) ([]byte, map[string][]byte) {
	t.Helper()
	contents := map[string][]byte{
		"config":                    []byte("# Managed by hand\r\nHost bastion\n\tPort 2222   \n"),
		"connections/work/lon.conf": []byte("Host lon-1\n\tHostName 203.0.113.11\n"),
		"sshc/metadata.json":        []byte(`{"schemaVersion":3}`),
		"keys/work/id_ed25519":      []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nnot really\n"),
	}
	manifest := remotesync.Manifest{
		CreatedAt: "2026-08-05T00:00:00Z",
		Origin:    "opaque-installation-id",
		Message:   "Test workspace snapshot",
		Files: []remotesync.Entry{
			entry("config", string(contents["config"])),
			entry("connections/work/lon.conf", string(contents["connections/work/lon.conf"])),
			entry("sshc/metadata.json", string(contents["sshc/metadata.json"])),
			entry("keys/work/id_ed25519", string(contents["keys/work/id_ed25519"])),
		},
	}
	if err := remotesync.FinalizeManifest(&manifest, ""); err != nil {
		t.Fatalf("FinalizeManifest = %v", err)
	}
	archive, err := remotesync.Build(manifest, contents)
	if err != nil {
		t.Fatalf("Build = %v", err)
	}
	return archive, contents
}

func TestRoundTripIsByteIdentical(t *testing.T) {
	// パーサの約束のすべてはバイト保存である。トランスポートがそれを壊すものであっては
	// ならないので、フィクスチャには CRLF、末尾の空白、そして末尾に改行のないファイルを
	// 含めてある。
	archive, contents := buildFixture(t)

	manifest, unpacked, err := remotesync.Read(archive)
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	if len(unpacked) != len(contents) {
		t.Fatalf("unpacked %d files, want %d", len(unpacked), len(contents))
	}
	for path, want := range contents {
		if !bytes.Equal(unpacked[path], want) {
			t.Errorf("%s round tripped as %q, want %q", path, unpacked[path], want)
		}
	}
	if manifest.SchemaVersion != remotesync.SchemaVersion {
		t.Errorf("schema version = %d", manifest.SchemaVersion)
	}
}

func TestRevisionIsStableAndCarriesItsParent(t *testing.T) {
	contents := map[string][]byte{"config": []byte("Host bastion\n")}
	manifest := remotesync.Manifest{
		CreatedAt: "2026-08-25T02:00:00Z", Origin: "opaque-installation-id",
		Message: "Add bastion connection",
		Files:   []remotesync.Entry{entry("config", string(contents["config"]))},
	}
	parent := strings.Repeat("a", 64)
	if err := remotesync.FinalizeManifest(&manifest, parent); err != nil {
		t.Fatal(err)
	}
	archive, err := remotesync.Build(manifest, contents)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := remotesync.Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got.ParentRevision != parent || len(got.Ancestors) != 1 || got.Ancestors[0] != parent ||
		got.Message != "Add bastion connection" || len(got.Revision) != 64 {
		t.Fatalf("manifest = %#v", got)
	}
	want, err := remotesync.RevisionFor(got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != want {
		t.Fatalf("revision = %q, want %q", got.Revision, want)
	}
}

func TestSnapshotRejectsAnInvalidAuthenticatedAncestorChain(t *testing.T) {
	parent := strings.Repeat("a", 64)
	manifest := remotesync.Manifest{
		CreatedAt: "2026-08-31T00:00:00Z", Origin: "origin", Message: "invalid lineage",
		ParentRevision: parent,
		Ancestors:      []string{parent, parent},
		Files:          []remotesync.Entry{entry("config", "Host test\n")},
	}
	manifest.Revision, _ = remotesync.RevisionFor(manifest)
	if _, err := remotesync.Build(manifest, map[string][]byte{"config": []byte("Host test\n")}); !errors.Is(err, remotesync.ErrManifestMismatch) {
		t.Fatalf("Build duplicate ancestors = %v, want ErrManifestMismatch", err)
	}
}

func TestCommitMessageParticipatesInRevisionAndRejectsInvalidText(t *testing.T) {
	manifest := remotesync.Manifest{CreatedAt: "2026-08-25T02:00:00Z", Origin: "origin", Message: "first", Files: []remotesync.Entry{}}
	first, err := remotesync.RevisionFor(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Message = "second"
	second, err := remotesync.RevisionFor(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("a changed commit message kept the same revision")
	}
	manifest.Message = "two\nlines"
	if err := remotesync.FinalizeManifest(&manifest, ""); !errors.Is(err, remotesync.ErrCommitMessage) {
		t.Fatalf("FinalizeManifest = %v, want ErrCommitMessage", err)
	}
	manifest.Message = "   "
	if err := remotesync.FinalizeManifest(&manifest, ""); !errors.Is(err, remotesync.ErrCommitMessage) {
		t.Fatalf("FinalizeManifest with blank message = %v, want ErrCommitMessage", err)
	}
}

func TestReadRefusesATamperedRevision(t *testing.T) {
	body := "Host bastion\n"
	archive := handBuilt(t, map[string]string{"config": body}, remotesync.Manifest{
		SchemaVersion: remotesync.SchemaVersion,
		CreatedAt:     "2026-08-25T02:00:00Z", Origin: "opaque-installation-id",
		Revision: strings.Repeat("0", 64),
		Files:    []remotesync.Entry{entry("config", body)},
	})
	if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrManifestMismatch) {
		t.Fatalf("Read = %v, want ErrManifestMismatch", err)
	}
}

func TestReadMigratesSchemaV5ToV6(t *testing.T) {
	parent := strings.Repeat("a", 64)
	manifest := remotesync.Manifest{
		SchemaVersion: 5,
		CreatedAt:     "2026-08-30T00:00:00Z", Origin: "v5-origin",
		ParentRevision: parent, Message: "Existing v5 snapshot",
		Files: []remotesync.Entry{entry("config", "Host migrated\n")},
	}
	manifest.Revision, _ = remotesync.RevisionFor(manifest)
	archive := handBuilt(t, map[string]string{"config": "Host migrated\n"}, manifest)

	migrated, _, err := remotesync.Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.SchemaVersion != 6 || migrated.Revision != manifest.Revision ||
		len(migrated.Ancestors) != 1 || migrated.Ancestors[0] != parent {
		t.Fatalf("migrated manifest = %#v", migrated)
	}
}

func TestReadRejectsAClaimedV5ManifestWithV6Fields(t *testing.T) {
	parent := strings.Repeat("a", 64)
	manifest := remotesync.Manifest{
		SchemaVersion: 5,
		CreatedAt:     "2026-08-30T00:00:00Z", Origin: "v5-origin",
		ParentRevision: parent, Ancestors: []string{parent}, Message: "Not really v5",
	}
	manifest.Revision, _ = remotesync.RevisionFor(manifest)
	archive := handBuilt(t, map[string]string{}, manifest)
	if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrNotASnapshot) {
		t.Fatalf("Read = %v, want ErrNotASnapshot", err)
	}
}

func TestReadRejectsUnsupportedSchemaVersions(t *testing.T) {
	versions := []int{1, 2, 3, 4, remotesync.SchemaVersion + 1}
	for _, version := range versions {
		archive := handBuilt(t, map[string]string{}, remotesync.Manifest{SchemaVersion: version})
		if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrUnsupportedVersion) {
			t.Errorf("schema %d: Read = %v, want ErrUnsupportedVersion", version, err)
		}
	}
}

func TestManifestCarriesNoHostname(t *testing.T) {
	archive, _ := buildFixture(t)
	manifest, _, err := remotesync.Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Origin == "" {
		t.Error("no origin at all, so the interface cannot say where a snapshot came from")
	}
	// このフィールドはインストールを区別するためにあり、マシンを指定するためではない。
	document, _ := json.Marshal(manifest)
	for _, forbidden := range []string{"hostname", "Hostname", ".local"} {
		if bytes.Contains(document, []byte(forbidden)) {
			t.Errorf("the manifest carries %q", forbidden)
		}
	}
}

func TestReadRefusesAPathThatEscapesTheWorkspace(t *testing.T) {
	// スナップショットは信用できない入力であり、tar の中の "../" は最も古い手口で
	// ある。
	for _, name := range []string{
		"../../etc/passwd",
		"/etc/passwd",
		"connections/../../outside.conf",
		"./config",
		"a//b",
		"..",
		"",
		`windows\\path`,
	} {
		t.Run(name, func(t *testing.T) {
			archive := handBuilt(t, map[string]string{name: "x"}, remotesync.Manifest{
				Files: []remotesync.Entry{{Path: name, SHA256: remotesync.Digest([]byte("x")), Mode: "0600"}},
			})
			if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrUnsafePath) {
				t.Fatalf("Read = %v, want ErrUnsafePath", err)
			}
		})
	}
}

func TestReadRefusesAPathWindowsCannotRepresent(t *testing.T) {
	for _, name := range []string{
		"CON",
		"prn.conf",
		"connections/AUX/key",
		"COM1.pub",
		"lpt9",
		"CONIN$",
		"conout$.log",
		"COM¹",
		"LPT³.txt",
		"config.",
		"config ",
		"connections./work.conf",
		"connections /work.conf",
	} {
		t.Run(name, func(t *testing.T) {
			archive := handBuilt(t, map[string]string{name: "x"}, remotesync.Manifest{
				Files: []remotesync.Entry{{Path: name, SHA256: remotesync.Digest([]byte("x")), Mode: "0600"}},
			})
			if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrUnsafePath) {
				t.Fatalf("Read = %v, want ErrUnsafePath", err)
			}
		})
	}
}

func TestWindowsDeviceNamePrefixesRemainUsable(t *testing.T) {
	for _, name := range []string{"console", "company/config", "com0", "com10", "lpt0", "lpt10", "auxiliary"} {
		t.Run(name, func(t *testing.T) {
			manifest := remotesync.Manifest{
				CreatedAt: "2026-08-31T00:00:00Z", Origin: "origin", Message: "Portable name",
				Files: []remotesync.Entry{entry(name, "x")},
			}
			if err := remotesync.FinalizeManifest(&manifest, ""); err != nil {
				t.Fatal(err)
			}
			archive, err := remotesync.Build(manifest, map[string][]byte{name: []byte("x")})
			if err != nil {
				t.Fatalf("Build = %v", err)
			}
			if _, _, err := remotesync.Read(archive); err != nil {
				t.Fatalf("Read = %v", err)
			}
		})
	}
}

func TestReadRefusesDeviceLocalAndRawVaultPaths(t *testing.T) {
	for _, name := range []string{
		"sshc/secrets",
		"sshc/sync-settings",
		"sshc/sync-state.json",
		"sshc/sync-key-recovery.json",
		"sshc/cli/request.json",
		"sshc/.cli.mutation.lock",
		"sshc/journal/20260830T000000.000000000Z-aaaaaaaaaaaaaaaa.json",
		"sshc/backups/foreign/config",
		"sshc/history/entry.json",
		"sshc/trash/deleted.conf",
		"sshc/recent-connections.json",
		"sshc/workspaces.json",
		"sshc/mutation.lock",
		"connections/.sshc-apply-temporary",
		remotesync.TravelPath + "/child",
		"SSHC/SNIPPETS.JSON",
	} {
		t.Run(name, func(t *testing.T) {
			archive := handBuilt(t, map[string]string{name: "attacker controlled"}, remotesync.Manifest{
				SchemaVersion: remotesync.SchemaVersion,
				CreatedAt:     "2026-08-30T00:00:00Z", Origin: "attacker", Message: "Reserved path",
				Files: []remotesync.Entry{entry(name, "attacker controlled")},
			})
			if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrUnsafePath) {
				t.Fatalf("Read = %v, want ErrUnsafePath", err)
			}
		})
	}
}

func TestReadAllowsOnlyBoundedLogicalProtectedDocuments(t *testing.T) {
	for _, name := range []string{remotesync.TravelPath, remotesync.SnippetsPath} {
		t.Run(name, func(t *testing.T) {
			body := "logical document"
			manifest := remotesync.Manifest{
				SchemaVersion: remotesync.SchemaVersion,
				CreatedAt:     "2026-08-30T00:00:00Z", Origin: "peer", Message: "Logical document",
				Files: []remotesync.Entry{entry(name, body)},
			}
			if err := remotesync.FinalizeManifest(&manifest, ""); err != nil {
				t.Fatal(err)
			}
			archive := handBuilt(t, map[string]string{name: body}, manifest)
			if _, _, err := remotesync.Read(archive); err != nil {
				t.Fatalf("Read = %v", err)
			}
		})
	}

	oversized := strings.Repeat("x", storage.MaxFileSize+1)
	archive := handBuilt(t, map[string]string{"oversized.conf": oversized}, remotesync.Manifest{
		SchemaVersion: remotesync.SchemaVersion,
		CreatedAt:     "2026-08-30T00:00:00Z", Origin: "peer", Message: "Oversized entry",
		Files: []remotesync.Entry{entry("oversized.conf", oversized)},
	})
	if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrSnapshotTooLarge) {
		t.Fatalf("Read oversized entry = %v, want ErrSnapshotTooLarge", err)
	}
}

func TestReadRefusesAModeThisApplicationDoesNotWrite(t *testing.T) {
	// 拒否する代わりに正規化すれば、スナップショットは秘密鍵の権限を、OpenSSH はまだ
	// 読むが他のユーザーも読める程度まで広げられてしまう。
	for _, mode := range []string{"0644", "0666", "0777", "0400", "", "not a mode"} {
		archive := handBuilt(t, map[string]string{"config": "x"}, remotesync.Manifest{
			Files: []remotesync.Entry{{Path: "config", SHA256: remotesync.Digest([]byte("x")), Mode: mode}},
		})
		if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrUnsafeMode) {
			t.Errorf("mode %q gave %v, want ErrUnsafeMode", mode, err)
		}
	}
}

func TestReadRefusesContentsThatDoNotMatchTheManifest(t *testing.T) {
	// マニフェストは、pull がローカルのディスクと比較する対象である。ファイルがそれと
	// 食い違うスナップショットは、ある一組のダイジェストから組み立てられ、別の一組から
	// 適用されるトランザクションを生んでしまう。
	archive := handBuilt(t, map[string]string{"config": "actual"}, remotesync.Manifest{
		Files: []remotesync.Entry{{Path: "config", SHA256: remotesync.Digest([]byte("claimed")), Mode: "0600"}},
	})
	if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrManifestMismatch) {
		t.Fatalf("Read = %v, want ErrManifestMismatch", err)
	}

	// アーカイブに存在するがマニフェストにないファイルは、同じ欠陥を逆方向から見たもの
	// である。それは検査されずに展開されてしまう。
	extra := handBuilt(t, map[string]string{"config": "x", "stowaway": "y"}, remotesync.Manifest{
		Files: []remotesync.Entry{{Path: "config", SHA256: remotesync.Digest([]byte("x")), Mode: "0600"}},
	})
	if _, _, err := remotesync.Read(extra); !errors.Is(err, remotesync.ErrManifestMismatch) {
		t.Fatalf("Read = %v, want ErrManifestMismatch", err)
	}
}

func TestReadRefusesSomethingThatIsNotASnapshot(t *testing.T) {
	cases := map[string][]byte{
		"empty":        {},
		"not gzip":     []byte("plain text"),
		"gzip only":    gzipOf(t, nil),
		"no manifest":  handBuiltWithoutManifest(t),
		"random bytes": bytes.Repeat([]byte{0x9f}, 512),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := remotesync.Read(input); err == nil {
				t.Fatal("Read succeeded")
			}
		})
	}
}

func TestBuildRefusesAnEntryWithNoContents(t *testing.T) {
	manifest := remotesync.Manifest{
		Message: "Test missing contents",
		Files:   []remotesync.Entry{{Path: "config", SHA256: "x", Mode: "0600"}},
	}
	if err := remotesync.FinalizeManifest(&manifest, ""); err != nil {
		t.Fatal(err)
	}
	_, err := remotesync.Build(manifest, map[string][]byte{})
	if !errors.Is(err, remotesync.ErrManifestMismatch) {
		t.Fatalf("Build = %v, want ErrManifestMismatch", err)
	}
}

func TestBuildRefusesContentsLargerThanTheReadLimit(t *testing.T) {
	body := bytes.Repeat([]byte{'x'}, remotesync.MaxSnapshotBytes+1)
	manifest := remotesync.Manifest{
		Message: "Test size limit",
		Files: []remotesync.Entry{{
			Path: "sshc/backgrounds/too-large.png", SHA256: remotesync.Digest(body), Mode: "0600",
		}},
	}
	if err := remotesync.FinalizeManifest(&manifest, ""); err != nil {
		t.Fatal(err)
	}
	_, err := remotesync.Build(manifest, map[string][]byte{"sshc/backgrounds/too-large.png": body})
	if !errors.Is(err, remotesync.ErrSnapshotTooLarge) {
		t.Fatalf("Build = %v, want ErrSnapshotTooLarge", err)
	}
}

func FuzzReadSnapshot(f *testing.F) {
	// Read は攻撃者由来の入力を解析する。アーカイブはバケットから来るのであり、その
	// バケットに書ける者が、その中身を選ぶ。
	archive, _ := buildFixture(&testing.T{})
	f.Add(archive)
	f.Add([]byte{})
	f.Add([]byte("not gzip at all"))

	f.Fuzz(func(t *testing.T, input []byte) {
		manifest, contents, err := remotesync.Read(input)
		if err != nil {
			return
		}
		// 解析を通るものはすべて、ファイルシステムに触れる前に、呼び出し側が依拠する
		// あらゆる不変条件を満たさなければならない。
		for _, item := range manifest.Files {
			if strings.HasPrefix(item.Path, "/") || strings.Contains(item.Path, "..") {
				t.Fatalf("an unsafe path survived: %q", item.Path)
			}
			if item.Mode != "0600" && item.Mode != "0700" {
				t.Fatalf("an unsafe mode survived: %q", item.Mode)
			}
			if remotesync.Digest(contents[item.Path]) != item.SHA256 {
				t.Fatalf("a digest mismatch survived for %q", item.Path)
			}
		}
		if len(contents) != len(manifest.Files) {
			t.Fatalf("%d files and %d manifest entries", len(contents), len(manifest.Files))
		}
	})
}

// handBuilt は Build を通さずにアーカイブを書く。これにより、Build なら拒否する
// ものをテストがその中に入れられる。
func handBuilt(t *testing.T, files map[string]string, manifest remotesync.Manifest) []byte {
	t.Helper()
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = remotesync.SchemaVersion
	}
	if manifest.Message == "" {
		manifest.Message = "Test snapshot"
	}
	document, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{remotesync.ManifestName: string(document)}
	for name, body := range files {
		entries[name] = body
	}
	return archiveOf(t, entries)
}

func handBuiltWithoutManifest(t *testing.T) []byte {
	t.Helper()
	return archiveOf(t, map[string]string{"config": "Host x\n"})
}

func archiveOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	zip := gzip.NewWriter(&compressed)
	archive := tar.NewWriter(zip)
	// まずマニフェスト、そのあとに他のすべて。Build が使うのと同じ順序である。
	if body, ok := entries[remotesync.ManifestName]; ok {
		writeRaw(t, archive, remotesync.ManifestName, body)
	}
	for name, body := range entries {
		if name == remotesync.ManifestName {
			continue
		}
		writeRaw(t, archive, name, body)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zip.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func writeRaw(t *testing.T, archive *tar.Writer, name, body string) {
	t.Helper()
	if err := archive.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg, Name: name, Mode: 0o600, Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
}

func gzipOf(t *testing.T, body []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zip := gzip.NewWriter(&buffer)
	if _, err := zip.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zip.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// スナップショットが保持してよいのはファイルだけである。
//
// tar の中のシンボリックリンクは、展開器に、自分が書いていると思っている場所の外へ
// 書かせる最も古い手口だ。リンクは内側に作られ、それを通して書かれる次のエントリが、
// リンクの指す先へ着地する。それを拒否する検査はずっとそこにあったのに、取り除かれた
// ときに誰も気づかなかった。それは、その検査を持っていないのと同じことで
// ある。
func TestOpenRefusesAnEntryThatIsNotAFile(t *testing.T) {
	for _, entry := range []struct {
		name   string
		header tar.Header
	}{
		{"symlink", tar.Header{Typeflag: tar.TypeSymlink, Name: "escape", Linkname: "../../../etc/hosts"}},
		{"hard link", tar.Header{Typeflag: tar.TypeLink, Name: "escape", Linkname: "config"}},
		{"directory", tar.Header{Typeflag: tar.TypeDir, Name: "connections/", Mode: 0o700}},
		{"device", tar.Header{Typeflag: tar.TypeChar, Name: "console", Mode: 0o600}},
	} {
		t.Run(entry.name, func(t *testing.T) {
			var compressed bytes.Buffer
			zip := gzip.NewWriter(&compressed)
			archive := tar.NewWriter(zip)
			document, err := json.Marshal(remotesync.Manifest{
				SchemaVersion: remotesync.SchemaVersion, CreatedAt: "2026-08-05T00:00:00Z",
				Origin: "o", Message: "Test unsafe archive entry", Files: []remotesync.Entry{},
			})
			if err != nil {
				t.Fatal(err)
			}
			writeRaw(t, archive, remotesync.ManifestName, string(document))
			if err := archive.WriteHeader(&entry.header); err != nil {
				t.Fatal(err)
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			if err := zip.Close(); err != nil {
				t.Fatal(err)
			}

			if _, _, err := remotesync.Read(compressed.Bytes()); !errors.Is(err, remotesync.ErrUnsafePath) {
				t.Errorf("a %s entry was accepted: %v", entry.name, err)
			}
		})
	}
}
