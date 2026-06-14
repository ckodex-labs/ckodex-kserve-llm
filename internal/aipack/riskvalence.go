package aipack

import v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"

// RiskSignalCount is the number of risk valence signals per AIPACK-SPEC v0.1.1 §13.2.
const RiskSignalCount = 13

// DefaultWeights are the reference signal weights from §13.2.
// Weights must sum to 100 (AIPACK-RV-001).
var DefaultWeights = [RiskSignalCount]int{
	10, // S1: vulnerability severity
	10, // S2: attestation completeness
	8,  // S3: provenance freshness
	8,  // S4: training data consent
	8,  // S5: license compliance
	8,  // S6: red-team score (inverted)
	8,  // S7: drift from baseline
	8,  // S8: deprecation proximity
	8,  // S9: dependency blast radius
	8,  // S10: sandbox escape risk
	6,  // S11: data residency compliance
	5,  // S12: cryptographic posture
	5,  // S13: outlier signal count
}

// rvBandThresholds maps band boundaries per §13.3.
// Score range [0,100]; higher = higher risk.
var rvBandThresholds = []struct {
	max  int
	band v1alpha2.RVBand
}{
	{24, v1alpha2.RVBandGreen},
	{49, v1alpha2.RVBandYellow},
	{74, v1alpha2.RVBandOrange},
	{100, v1alpha2.RVBandRed},
}

// ComputeRiskValence computes the risk valence score and band per AIPACK-SPEC §13.
// signals contains the normalised [0,1] value for each of the 13 signals (0=low risk, 1=high risk).
// weights contains the integer weights that must sum to 100 (AIPACK-RV-001).
func ComputeRiskValence(signals [RiskSignalCount]float64, weights [RiskSignalCount]int) (score int, band v1alpha2.RVBand, err error) {
	// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §13 — weighted sum formula
	sum := 0
	for _, w := range weights {
		sum += w
	}
	if sum != 100 {
		return 0, "", newErr(ErrRVWeightsSumInvalid,
			"risk valence weights must sum to 100 (AIPACK-RV-001)",
			"",
		)
	}
	raw := 0.0
	for i := range signals {
		raw += signals[i] * float64(weights[i])
	}
	score = int(raw)
	if score > 100 {
		score = 100
	}
	band = bandForScore(score)
	return score, band, nil
}

// BandForScore maps a risk valence score to its RVBand per §13.3.
// Exported for use in conformance tests and tooling.
func BandForScore(score int) v1alpha2.RVBand { return bandForScore(score) }

func bandForScore(score int) v1alpha2.RVBand {
	for _, t := range rvBandThresholds {
		if score <= t.max {
			return t.band
		}
	}
	return v1alpha2.RVBandRed
}
