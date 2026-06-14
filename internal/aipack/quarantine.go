package aipack

// QuarantineTrigger is an §21 trigger declaration that fires when a predicate condition
// evaluates to true at admission or reconciliation time.
// Backed by attestation urn:aipack:trigger-fired:v1.
type QuarantineTrigger struct {
	// ID is a unique identifier for this trigger declaration.
	ID string `json:"id"`

	// Name is a human-readable trigger name.
	Name string `json:"name"`

	// Condition is a predicate DSL expression (see §21.2 for DSL grammar).
	Condition string `json:"condition"`

	// Action declares the action to take when the trigger fires.
	// Values: "quarantine", "block", "alert", "log"
	Action string `json:"action"`

	// Severity is the trigger severity level.
	// Values: "critical", "high", "medium", "low"
	Severity string `json:"severity"`
}

// StandardTriggers is the library of standard quarantine triggers from §21.3.
var StandardTriggers = []QuarantineTrigger{
	{
		ID:        "QT-001",
		Name:      "RED-band composition block",
		Condition: "riskBand == RED && !hasAttestation(urn:aipack:profile-derogation:v1)",
		Action:    "block",
		Severity:  "critical",
	},
	{
		ID:        "QT-002",
		Name:      "Missing required predicate at staging promotion",
		Condition: "environment == staging && missingRequiredPredicates.length > 0",
		Action:    "block",
		Severity:  "high",
	},
	{
		ID:        "QT-003",
		Name:      "Sunset date exceeded",
		Condition: "deprecationPhase == sunset && now() > sunsetDate",
		Action:    "quarantine",
		Severity:  "critical",
	},
}

// EvaluateTrigger evaluates a single trigger condition against the given context map.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §21 — DSL expression evaluator
func EvaluateTrigger(_ *QuarantineTrigger, _ map[string]interface{}) (bool, error) {
	return false, newErr(ErrQuarantineTriggerFired, "trigger evaluation not yet implemented", "")
}
