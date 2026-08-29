/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package provenance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
)

const EvidenceReceiptSchemaV1 = "ckodex.evidence.receipt/v1"

var (
	ErrReceiptShape     = errors.New("invalid evidence receipt shape")
	ErrReceiptDigest    = errors.New("invalid evidence receipt digest")
	ErrReceiptProducer  = errors.New("invalid evidence receipt producer binding")
	ErrReceiptSignature = errors.New("invalid evidence receipt signature")
)

// ProducerBinding identifies the workload and trusted key that produced a
// receipt. The public key is supplied by the verifier's trust configuration;
// it is not accepted from the receipt itself.
type ProducerBinding struct {
	SPIFFEID  string `json:"spiffeId"`
	KeyDigest string `json:"keyDigest"`
}

// EvidenceReceipt is a content-free, signed reference to evidence. It cannot
// carry prompts, model output, retrieved documents, or arbitrary detail maps.
// Signature proves the configured producer key signed Digest; it does not
// assert transparency-log inclusion.
type EvidenceReceipt struct {
	Schema         string          `json:"schema"`
	ID             string          `json:"id"`
	SubjectDigest  string          `json:"subjectDigest"`
	Producer       ProducerBinding `json:"producer"`
	Sequence       uint64          `json:"sequence"`
	PreviousDigest string          `json:"previousDigest,omitempty"`
	ProducedAt     string          `json:"producedAt"`
	Digest         string          `json:"digest"`
	Signature      string          `json:"signature"`
}

type receiptClaims struct {
	Schema         string          `json:"schema"`
	ID             string          `json:"id"`
	SubjectDigest  string          `json:"subjectDigest"`
	Producer       ProducerBinding `json:"producer"`
	Sequence       uint64          `json:"sequence"`
	PreviousDigest string          `json:"previousDigest,omitempty"`
	ProducedAt     string          `json:"producedAt"`
}

// ReceiptVerifier verifies receipts against operator-configured producer keys.
type ReceiptVerifier struct {
	trusted map[string]ed25519.PublicKey
}

// NewReceiptVerifier copies and validates the supplied SPIFFE-to-key trust map.
func NewReceiptVerifier(trusted map[string]ed25519.PublicKey) (*ReceiptVerifier, error) {
	verifier := &ReceiptVerifier{trusted: make(map[string]ed25519.PublicKey, len(trusted))}
	for producerID, publicKey := range trusted {
		if _, err := spiffeid.FromString(producerID); err != nil {
			return nil, fmt.Errorf("%w: producer %q: %w", ErrReceiptProducer, producerID, err)
		}
		if len(publicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: producer %q has invalid Ed25519 public key", ErrReceiptProducer, producerID)
		}
		verifier.trusted[producerID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	return verifier, nil
}

// SignEvidenceReceipt builds a receipt signed by an explicitly supplied
// Ed25519 key. Key acquisition and transparency-log publication are outside
// this contract and must be evidenced separately.
func SignEvidenceReceipt(receipt EvidenceReceipt, privateKey ed25519.PrivateKey) (EvidenceReceipt, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return EvidenceReceipt{}, fmt.Errorf("%w: invalid Ed25519 private key", ErrReceiptProducer)
	}
	receipt.Schema = EvidenceReceiptSchemaV1
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt.Producer.KeyDigest = digestBytes(publicKey)
	digest, err := receiptClaimsDigest(receipt)
	if err != nil {
		return EvidenceReceipt{}, err
	}
	receipt.Digest = digest
	receipt.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(digest)))
	return receipt, nil
}

// Verify checks shape, content commitment, configured producer binding, and
// Ed25519 signature. It does not verify Rekor or another transparency service.
func (v *ReceiptVerifier) Verify(receipt EvidenceReceipt) error {
	if err := validateReceiptShape(receipt); err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("%w: verifier is nil", ErrReceiptProducer)
	}
	publicKey, ok := v.trusted[receipt.Producer.SPIFFEID]
	if !ok {
		return fmt.Errorf("%w: producer %q is not trusted", ErrReceiptProducer, receipt.Producer.SPIFFEID)
	}
	if receipt.Producer.KeyDigest != digestBytes(publicKey) {
		return fmt.Errorf("%w: key digest does not match configured producer key", ErrReceiptProducer)
	}
	expectedDigest, err := receiptClaimsDigest(receipt)
	if err != nil {
		return err
	}
	if receipt.Digest != expectedDigest {
		return fmt.Errorf("%w: claims digest mismatch", ErrReceiptDigest)
	}
	signature, err := base64.StdEncoding.DecodeString(receipt.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: malformed Ed25519 signature", ErrReceiptSignature)
	}
	if !ed25519.Verify(publicKey, []byte(receipt.Digest), signature) {
		return ErrReceiptSignature
	}
	return nil
}

// ParseEvidenceReceipt rejects unknown fields so content-bearing extensions
// cannot silently enter the receipt plane.
func ParseEvidenceReceipt(raw []byte) (EvidenceReceipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var receipt EvidenceReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return EvidenceReceipt{}, fmt.Errorf("%w: %w", ErrReceiptShape, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return EvidenceReceipt{}, err
	}
	if err := validateReceiptShape(receipt); err != nil {
		return EvidenceReceipt{}, err
	}
	return receipt, nil
}

func validateReceiptShape(receipt EvidenceReceipt) error {
	if receipt.Schema != EvidenceReceiptSchemaV1 || receipt.ID == "" || receipt.Sequence == 0 {
		return fmt.Errorf("%w: schema, id, and positive sequence are required", ErrReceiptShape)
	}
	if _, err := spiffeid.FromString(receipt.Producer.SPIFFEID); err != nil {
		return fmt.Errorf("%w: invalid SPIFFE ID", ErrReceiptProducer)
	}
	if !validSHA256(receipt.SubjectDigest) || !validSHA256(receipt.Producer.KeyDigest) || !validSHA256(receipt.Digest) {
		return fmt.Errorf("%w: sha256 references must use 64 lowercase hex characters", ErrReceiptDigest)
	}
	if receipt.Sequence == 1 && receipt.PreviousDigest != "" {
		return fmt.Errorf("%w: first receipt cannot reference a predecessor", ErrReceiptShape)
	}
	if receipt.Sequence > 1 && !validSHA256(receipt.PreviousDigest) {
		return fmt.Errorf("%w: chained receipt requires a predecessor digest", ErrReceiptDigest)
	}
	producedAt, err := time.Parse(time.RFC3339Nano, receipt.ProducedAt)
	if err != nil || producedAt.Location() != time.UTC {
		return fmt.Errorf("%w: producedAt must be an RFC3339 UTC timestamp", ErrReceiptShape)
	}
	if receipt.Signature == "" {
		return fmt.Errorf("%w: signature is required", ErrReceiptSignature)
	}
	return nil
}

func receiptClaimsDigest(receipt EvidenceReceipt) (string, error) {
	claims := receiptClaims{
		Schema: receipt.Schema, ID: receipt.ID, SubjectDigest: receipt.SubjectDigest,
		Producer: receipt.Producer, Sequence: receipt.Sequence,
		PreviousDigest: receipt.PreviousDigest, ProducedAt: receipt.ProducedAt,
	}
	encoded, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal receipt claims: %w", err)
	}
	return digestBytes(encoded), nil
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: trailing JSON value", ErrReceiptShape)
		}
		return fmt.Errorf("%w: %w", ErrReceiptShape, err)
	}
	return nil
}
