/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// RetrievalIndexSpec describes an A9 RetrievalIndex artifact per AIPACK-SPEC v0.1.1 §3.3.
type RetrievalIndexSpec struct {
	// EmbeddingModel is the OCI digest reference to the BaseModel used to generate embeddings.
	// Required for slot compatibility validation (AIPACK-COMPAT-002).
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	EmbeddingModel string `json:"embeddingModel"`

	// IndexType declares the vector index type.
	// Examples: "hnsw", "ivfpq", "flat", "lsh"
	// +optional
	IndexType string `json:"indexType,omitempty"`

	// DocumentCount is the number of documents indexed.
	// +optional
	DocumentCount *int64 `json:"documentCount,omitempty"`

	// VectorDimension is the embedding vector dimension.
	// Must match EmbeddingModel.EmbeddingDimension when resolvable.
	// +optional
	VectorDimension *int32 `json:"vectorDimension,omitempty"`

	// Language declares the primary language(s) of the indexed content.
	// +optional
	Language []string `json:"language,omitempty"`

	// PIIScanPassed declares whether a PII scan has been completed.
	// Backed by attestation urn:retrieval:pii-scan:v1.
	// +optional
	PIIScanPassed *bool `json:"piiScanPassed,omitempty"`

	// RefreshCadence describes how frequently the index is refreshed.
	// Examples: "daily", "weekly", "on-change", "manual"
	// +optional
	RefreshCadence string `json:"refreshCadence,omitempty"`

	// ByteSize is the total compressed index size in bytes.
	// +optional
	ByteSize *int64 `json:"byteSize,omitempty"`
}

// DatasetSpec describes an A10 Dataset artifact per AIPACK-SPEC v0.1.1 §3.3.
type DatasetSpec struct {
	// Format declares the dataset format.
	// Examples: "jsonl", "parquet", "csv", "tfrecord", "arrow"
	// +optional
	Format string `json:"format,omitempty"`

	// RecordCount is the number of records in the dataset.
	// +optional
	RecordCount *int64 `json:"recordCount,omitempty"`

	// ByteSize is the uncompressed dataset size in bytes.
	// +optional
	ByteSize *int64 `json:"byteSize,omitempty"`

	// Language declares the primary language(s) of the dataset content.
	// +optional
	Language []string `json:"language,omitempty"`

	// License is the dataset license identifier.
	// Examples: "Apache-2.0", "CC-BY-4.0", "CC-BY-NC-4.0", "proprietary"
	// +optional
	License string `json:"license,omitempty"`

	// ConsentVerified declares whether appropriate consent has been verified.
	// Backed by attestation urn:dataset:consent:v1.
	// +optional
	ConsentVerified *bool `json:"consentVerified,omitempty"`

	// Deidentified declares whether PII has been removed or anonymized.
	// Backed by attestation urn:dataset:deidentification:v1.
	// +optional
	Deidentified *bool `json:"deidentified,omitempty"`

	// BiasAnalysisRefs lists URIs pointing to bias analysis reports.
	// Backed by attestation urn:dataset:bias-analysis:v1.
	// +optional
	BiasAnalysisRefs []string `json:"biasAnalysisRefs,omitempty"`

	// DataCutoff is the ISO 8601 date of the most recent data in the dataset.
	// +optional
	DataCutoff string `json:"dataCutoff,omitempty"`

	// ProvenanceSources lists source descriptions for the dataset.
	// Backed by attestation urn:dataset:provenance:v1.
	// +optional
	ProvenanceSources []string `json:"provenanceSources,omitempty"`
}
