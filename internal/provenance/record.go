package provenance

import "encoding/json"

// RuntimeVerificationRecord captures the outcome of a storage-initializer
// verification run in a format that can be written to the termination log and
// consumed by controllers.
type RuntimeVerificationRecord struct {
	Subject             string `json:"subject,omitempty"`
	Scheme              string `json:"scheme,omitempty"`
	SignatureVerified   bool   `json:"signatureVerified,omitempty"`
	AttestationVerified bool   `json:"attestationVerified,omitempty"`
	SBOMVerified        bool   `json:"sbomVerified,omitempty"`
	SignatureDigest     string `json:"signatureDigest,omitempty"`
	AttestationURI      string `json:"attestationUri,omitempty"`
	SBOMDigest          string `json:"sbomDigest,omitempty"`
	KeyRef              string `json:"keyRef,omitempty"`
	CertificateIdentity string `json:"certificateIdentity,omitempty"`
	CertificateIssuer   string `json:"certificateIssuer,omitempty"`
	VerifiedAt          string `json:"verifiedAt,omitempty"`
	Error               string `json:"error,omitempty"`
}

// Verified reports whether every proof surface required by the runtime policy
// completed successfully.
func (r RuntimeVerificationRecord) Verified() bool {
	return r.Error == "" &&
		r.SignatureVerified &&
		r.AttestationVerified &&
		r.SBOMVerified &&
		r.SignatureDigest != "" &&
		r.AttestationURI != "" &&
		r.SBOMDigest != ""
}

// ParseRuntimeVerificationRecord decodes a termination-log payload.
func ParseRuntimeVerificationRecord(raw string) (*RuntimeVerificationRecord, error) {
	var record RuntimeVerificationRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, err
	}
	return &record, nil
}
