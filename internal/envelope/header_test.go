package envelope

import "testing"

// 下げたコストはヘッダーに書かれ、開く側はそのコストで開く。ほかのパッケージの
// テストは、これを頼りに製品と同じ Open を安く通している。
func TestALoweredDerivationCostIsWrittenToTheHeader(t *testing.T) {
	lowered := Limits{Time: 1, MemoryKiB: 64, Threads: 1}
	previous := DerivationCost
	DerivationCost = lowered
	t.Cleanup(func() { DerivationCost = previous })

	key, err := Derive("a passphrase long enough")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Seal([]byte("a snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	_, params, _, err := readHeader(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if params.Time != lowered.Time || params.Memory != lowered.MemoryKiB || params.Threads != lowered.Threads {
		t.Fatalf("header cost = %d/%d KiB/%d threads, want %+v", params.Time, params.Memory, params.Threads, lowered)
	}
	plaintext, _, err := Open(sealed, "a passphrase long enough")
	if err != nil {
		t.Fatalf("Open with the lowered cost = %v", err)
	}
	if string(plaintext) != "a snapshot" {
		t.Errorf("plaintext = %q", plaintext)
	}
}
