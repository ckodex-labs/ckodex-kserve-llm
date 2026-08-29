/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package provenance

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const testProducerID = "spiffe://ckodex.com/ns/default/sa/operator"

func receiptTestKey(fill byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = fill
	}
	return ed25519.NewKeyFromSeed(seed)
}

func receiptTestSubject(value string) string {
	digest := sha256.Sum256([]byte(value))
	return digestBytes(digest[:])
}

func signedTestReceipt(t *testing.T, key ed25519.PrivateKey) EvidenceReceipt {
	t.Helper()
	receipt, err := SignEvidenceReceipt(EvidenceReceipt{
		ID: "receipt-1", SubjectDigest: receiptTestSubject("subject"),
		Producer: ProducerBinding{SPIFFEID: testProducerID}, Sequence: 1,
		ProducedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}, key)
	if err != nil {
		t.Fatalf("sign receipt: %v", err)
	}
	return receipt
}

func TestReceiptSpecToRuntime_VerifiesContentCommitmentAndProducerBinding(t *testing.T) {
	key := receiptTestKey(1)
	receipt := signedTestReceipt(t, key)
	verifier, err := NewReceiptVerifier(map[string]ed25519.PublicKey{testProducerID: key.Public().(ed25519.PublicKey)})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	if err := verifier.Verify(receipt); err != nil {
		t.Fatalf("verify receipt: %v", err)
	}

	tampered := receipt
	tampered.SubjectDigest = receiptTestSubject("different subject")
	if err := verifier.Verify(tampered); !errors.Is(err, ErrReceiptDigest) {
		t.Fatalf("tampered digest error = %v, want ErrReceiptDigest", err)
	}

	wrongProducer := receipt
	wrongProducer.Producer.SPIFFEID = "spiffe://ckodex.com/ns/other/sa/operator"
	if err := verifier.Verify(wrongProducer); !errors.Is(err, ErrReceiptProducer) {
		t.Fatalf("producer binding error = %v, want ErrReceiptProducer", err)
	}
}

func TestReceiptRuntimeToSpec_SerializationCannotCarryContent(t *testing.T) {
	receipt := signedTestReceipt(t, receiptTestKey(2))
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	for _, forbidden := range []string{"prompt", "output", "messages", "content", "details"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("receipt contains forbidden content field %q: %s", forbidden, encoded)
		}
	}
	parsed, err := ParseEvidenceReceipt(encoded)
	if err != nil {
		t.Fatalf("parse receipt: %v", err)
	}
	if parsed.SubjectDigest != receipt.SubjectDigest {
		t.Fatalf("subject digest = %q, want %q", parsed.SubjectDigest, receipt.SubjectDigest)
	}

	withContent := append(encoded[:len(encoded)-1], []byte(`,"prompt":"secret"}`)...)
	if _, err := ParseEvidenceReceipt(withContent); !errors.Is(err, ErrReceiptShape) {
		t.Fatalf("content-bearing receipt error = %v, want ErrReceiptShape", err)
	}
}

func TestReceiptSpecToRuntime_RejectsUntrustedKeyAndInvalidSignature(t *testing.T) {
	receipt := signedTestReceipt(t, receiptTestKey(3))
	otherKey := receiptTestKey(4)
	verifier, err := NewReceiptVerifier(map[string]ed25519.PublicKey{testProducerID: otherKey.Public().(ed25519.PublicKey)})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	if err := verifier.Verify(receipt); !errors.Is(err, ErrReceiptProducer) {
		t.Fatalf("key binding error = %v, want ErrReceiptProducer", err)
	}

	trustedVerifier, err := NewReceiptVerifier(map[string]ed25519.PublicKey{testProducerID: receiptTestKey(3).Public().(ed25519.PublicKey)})
	if err != nil {
		t.Fatalf("new trusted verifier: %v", err)
	}
	receipt.Signature = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
	if err := trustedVerifier.Verify(receipt); !errors.Is(err, ErrReceiptSignature) {
		t.Fatalf("signature error = %v, want ErrReceiptSignature", err)
	}
}

func TestReceiptRuntimeToSpec_DoesNotClaimTransparencyInclusion(t *testing.T) {
	receipt := signedTestReceipt(t, receiptTestKey(5))
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	for _, unsupported := range []string{"rekor", "inclusion", "transparency"} {
		if strings.Contains(strings.ToLower(string(encoded)), unsupported) {
			t.Fatalf("receipt claims unsupported %q evidence: %s", unsupported, encoded)
		}
	}
}
