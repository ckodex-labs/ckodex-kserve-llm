package aipack

// OutlierCategory classifies the type of statistical outlier per AIPACK-SPEC v0.1.1 §14.
type OutlierCategory string

const (
	OutlierCategoryPerformance  OutlierCategory = "performance"
	OutlierCategoryBehavioral   OutlierCategory = "behavioral"
	OutlierCategoryDistribution OutlierCategory = "distribution"
	OutlierCategoryProvenance   OutlierCategory = "provenance"
)

// OutlierSignal is the §14 outlier detection result.
// Backed by attestation urn:aipack:outlier-signal:v1.
type OutlierSignal struct {
	// ArtifactRef is the artifact in which the outlier was detected.
	ArtifactRef string `json:"artifactRef"`

	// Category is the outlier classification.
	Category OutlierCategory `json:"category"`

	// Severity is the outlier severity in range [0,1].
	Severity float64 `json:"severity"`

	// Dimension describes the measurement dimension in which the outlier was detected.
	Dimension string `json:"dimension"`

	// DetectedAt is the RFC 3339 timestamp of detection.
	DetectedAt string `json:"detectedAt"`

	// Acknowledged records whether the signal has been reviewed and dismissed.
	Acknowledged bool `json:"acknowledged"`
}

// DetectOutliers computes outlier signals for the given artifact metrics.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §14 — statistical analysis + thresholding
func DetectOutliers(_ string, _ map[string]float64) ([]OutlierSignal, error) {
	return nil, newErr(ErrOutlierUnacknowledged, "outlier detection not yet implemented", "")
}

// DismissOutlier records a dismissal decision for an acknowledged outlier signal.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §14 — emit urn:aipack:outlier-dismissal:v1
func DismissOutlier(_ *OutlierSignal, _ string) error {
	return newErr(ErrOutlierUnacknowledged, "outlier dismissal not yet implemented", "")
}
