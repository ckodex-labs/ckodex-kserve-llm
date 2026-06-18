package aipack

// VADClass is the Vulnerability/Adversarial Defense class per AIPACK-SPEC v0.1.1 §22.
type VADClass string

const (
	VADClassPromptInjection     VADClass = "prompt-injection"
	VADClassJailbreak           VADClass = "jailbreak"
	VADClassDataPoisoning       VADClass = "data-poisoning"
	VADClassModelInversion      VADClass = "model-inversion"
	VADClassMembershipInference VADClass = "membership-inference"
	VADClassAdversarialInput    VADClass = "adversarial-input"
)

// VADDeclaration records a §22 VAD test result for an artifact.
// Backed by attestation urn:aipack:vad-result:v1.
type VADDeclaration struct {
	// ArtifactRef is the OCI digest reference of the tested artifact.
	ArtifactRef string `json:"artifactRef"`

	// Classes lists the VAD classes tested.
	Classes []VADClass `json:"classes"`

	// PerturbationFamilies lists the perturbation families applied.
	PerturbationFamilies []string `json:"perturbationFamilies"`

	// Passed reports whether all tested classes passed.
	Passed bool `json:"passed"`

	// Score is the aggregate VAD score in [0,1] (1 = fully defended).
	Score float64 `json:"score"`

	// FailedClasses lists the VAD classes that failed.
	FailedClasses []VADClass `json:"failedClasses,omitempty"`
}

// RunVAD executes a VAD evaluation for the given artifact ref and VAD classes.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §22 — apply perturbation families + evaluate
func RunVAD(_ string, _ []VADClass) (*VADDeclaration, error) {
	return nil, newErr(ErrVADConsensusFailed, "VAD evaluation not yet implemented", "")
}

// ValidateVADDeclaration validates that a VADDeclaration covers the required classes for kind.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §22 — class coverage check per kind
func ValidateVADDeclaration(_ *VADDeclaration) error {
	return nil
}
