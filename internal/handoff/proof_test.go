package handoff_test

import (
	"bytes"
	"testing"

	"sshc/internal/handoff"
)

func TestProofBindsTheSecretToTheChallenge(t *testing.T) {
	challenge, err := handoff.MintChallenge(bytes.NewReader(bytes.Repeat([]byte{7}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	if !handoff.ValidChallenge(challenge) {
		t.Fatalf("minted challenge %q is not valid", challenge)
	}
	proof := handoff.Prove("the secret", challenge)
	if !handoff.VerifyProof("the secret", challenge, proof) {
		t.Fatal("the engine's own proof was refused")
	}
	if handoff.VerifyProof("another secret", challenge, proof) {
		t.Fatal("a proof made with another secret was accepted")
	}
	if handoff.VerifyProof("the secret", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", proof) {
		t.Fatal("a proof for another challenge was accepted")
	}
	if handoff.VerifyProof("the secret", challenge, "") || handoff.ValidChallenge("short") {
		t.Fatal("malformed values were accepted")
	}
}
