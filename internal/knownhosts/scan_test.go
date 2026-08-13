package knownhosts_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/platform"
)

// recordingCollector は、鍵を集める継ぎ目を差し替える。
//
// 本物の握手を見るのは internal/sshclient の側である。ここが確かめるのは、
// 安全でない宛先を継ぎ目より前で拒むことと、集めた鍵が候補の形になることである。
type recordingCollector struct {
	addresses []string
	keys      []ssh.PublicKey
	err       error
}

func (c *recordingCollector) collect(
	_ context.Context, address string, _ time.Duration,
) ([]ssh.PublicKey, error) {
	c.addresses = append(c.addresses, address)
	return c.keys, c.err
}

func fixturePublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	blob, err := base64.StdEncoding.DecodeString(fixtureKey)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.ParsePublicKey(blob)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestScanReturnsUnverifiedCandidates(t *testing.T) {
	collector := &recordingCollector{keys: []ssh.PublicKey{fixturePublicKey(t)}}

	candidates, err := knownhosts.Scanner{Collect: collector.collect}.
		Scan(context.Background(), "bastion.example.com", 2222)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v", candidates)
	}
	candidate := candidates[0]
	if candidate.Verified {
		t.Error("a scanned key is never verified")
	}
	if candidate.Fingerprint != fixtureFingerprint || candidate.Port != 2222 {
		t.Errorf("candidate = %#v", candidate)
	}
	if candidate.KeyType != fixtureKeyType || candidate.Key != fixtureKey {
		t.Errorf("candidate = %#v", candidate)
	}
	if len(collector.addresses) != 1 || collector.addresses[0] != "bastion.example.com:2222" {
		t.Errorf("addresses = %#v", collector.addresses)
	}
	if !strings.Contains(knownhosts.UnverifiedNotice, "does not prove") {
		t.Error("the notice must say what a scan does not prove")
	}
}

// 宛先の検査は継ぎ目より前にある。安全でない値でそこへ届いてはならない。
func TestScanRejectsUnsafeTargetsBeforeReachingTheNetwork(t *testing.T) {
	collector := &recordingCollector{}
	scanner := knownhosts.Scanner{Collect: collector.collect}

	if _, err := scanner.Scan(context.Background(), "-p2222", 22); !errors.Is(err, platform.ErrUnsafeHostname) {
		t.Fatalf("unsafe host = %v, want ErrUnsafeHostname", err)
	}
	if _, err := scanner.Scan(context.Background(), "example.com", 0); !errors.Is(err, platform.ErrUnsafePort) {
		t.Fatalf("invalid port = %v, want ErrUnsafePort", err)
	}
	if len(collector.addresses) != 0 {
		t.Fatal("an unsafe target reached the network")
	}
}

// 集める手段が無いレジストリは、集めたふりをしない。
func TestScanWithoutACollectorRefuses(t *testing.T) {
	if _, err := (knownhosts.Scanner{}).Scan(
		context.Background(), "example.com", 22,
	); !errors.Is(err, knownhosts.ErrNoScanner) {
		t.Fatalf("Scan = %v, want ErrNoScanner", err)
	}
}
