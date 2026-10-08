/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Canonical target identifiers for conformance vectors.
const (
	TargetGLM53EXL3TensorFold = "GLM-5.3-EXL3-4x-RTX-PRO-6000-TensorFold"
	TargetVLLMNVFP4FlashMLA   = "vLLM-v0.31.0-NVFP4-FlashMLA"
	TargetDecisionEvaluator   = "DecisionEvaluator"
)

// Standard12ClassSuite returns a suite containing vectors covering all 12 Conformance Classes.
func Standard12ClassSuite() (*ConformanceSuite, error) {
	suite := NewConformanceSuite()
	vectors := []ConformanceVector{
		createPositiveGLMVector(),
		createPositiveVLLMVector(),
		createPositiveEvaluatorVector(),
		createNegativeVLLMVector(),
		createNegativeEvaluatorVector(),
		createAntiGLMTamperVector(),
		createAntiVLLMBypassVector(),
		createAntiEvaluatorForbiddenTupleVector(),
		createTemporalExpiryVector(),
		createDegradationFlashMLAVector(),
		createRecoveryTensorFoldVector(),
		createRaceFlashMLAVector(),
		createMetamorphicEvaluatorVector(),
		createSaturationGLMVector(),
		createValidButWrongFoldVector(),
		createSilenceMissingReceiptVector(),
		createCounterfactualReplayVector(),
	}

	if err := suite.RegisterMany(vectors...); err != nil {
		return nil, fmt.Errorf("failed to register standard vectors: %w", err)
	}
	return suite, nil
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
