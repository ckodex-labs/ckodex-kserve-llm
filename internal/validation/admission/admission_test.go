/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package admission_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckodex-labs/kserve-llm-operator/internal/validation/admission"
)

func newTestEngine(t *testing.T) (*admission.Engine, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	producer := admission.ProducerBinding{
		SPIFFEID:  "spiffe://ckodex.internal/ns/system/sa/admission-engine",
		KeyDigest: "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
	}
	return admission.NewEngine(producer, admission.WithPrivateKey(priv)), pub
}

type dataCase struct {
	name     string
	input    admission.DataAdmissionInput
	wantDisp admission.Disposition
}

func runDataCases(t *testing.T, cases []dataCase) {
	t.Helper()
	engine, _ := newTestEngine(t)
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := engine.AdmitData(ctx, tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDisp, dec.Disposition)
		})
	}
}

func TestStage1_DataAdmission_AdmitAndConditional(t *testing.T) {
	engine, _ := newTestEngine(t)
	ctx := context.Background()

	t.Run("admit", func(t *testing.T) {
		dec, err := engine.AdmitData(ctx, admission.DataAdmissionInput{
			SubjectID: "ds-01", ProvenanceState: admission.StateVerified,
			LicensingState: admission.StateCompliant, PrivacyState: admission.StateCompliant,
			IntegrityState: admission.StateVerified,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmit, dec.Disposition)
		assert.Empty(t, dec.Obligations)
	})

	t.Run("admit_with_obligations", func(t *testing.T) {
		dec, err := engine.AdmitData(ctx, admission.DataAdmissionInput{
			SubjectID: "ds-02", ProvenanceState: admission.StateVerified,
			LicensingState: admission.StateConditional, PrivacyState: admission.StateCompliant,
			IntegrityState: admission.StateVerified,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmitWithObligations, dec.Disposition)
		assert.NotEmpty(t, dec.Obligations)
	})
}

func TestStage1_DataAdmission_NonAdmitting(t *testing.T) {
	base := admission.DataAdmissionInput{
		SubjectID: "ds-test", ProvenanceState: admission.StateVerified,
		LicensingState: admission.StateCompliant, PrivacyState: admission.StateCompliant,
		IntegrityState: admission.StateVerified,
	}
	denyIn, escIn, safeIn, quarIn := base, base, base, base
	denyIn.ProvenanceState = admission.StateViolated
	escIn.ProvenanceState = admission.StateUnknown
	safeIn.IntegrityState = admission.StatePending
	quarIn.IntegrityState = admission.StateQuarantined

	runDataCases(t, []dataCase{
		{name: "deny", input: denyIn, wantDisp: admission.DispositionDeny},
		{name: "escalate", input: escIn, wantDisp: admission.DispositionEscalate},
		{name: "safe_hold", input: safeIn, wantDisp: admission.DispositionSafeHold},
		{name: "quarantine", input: quarIn, wantDisp: admission.DispositionQuarantine},
	})
}

type modelCase struct {
	name     string
	input    admission.ModelAdmissionInput
	wantDisp admission.Disposition
}

func runModelCases(t *testing.T, cases []modelCase) {
	t.Helper()
	engine, _ := newTestEngine(t)
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := engine.AdmitModel(ctx, tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDisp, dec.Disposition)
		})
	}
}

func TestStage2_ModelAdmission_AdmitAndConditional(t *testing.T) {
	engine, _ := newTestEngine(t)
	ctx := context.Background()

	t.Run("admit", func(t *testing.T) {
		dec, err := engine.AdmitModel(ctx, admission.ModelAdmissionInput{
			SubjectID: "model-01", ArtifactIntegrityState: admission.StateVerified,
			SBOMState: admission.StateVerified, SignatureState: admission.StateVerified,
			AttestationState: admission.StateVerified, LicenseState: admission.StateCompliant,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmit, dec.Disposition)
	})

	t.Run("admit_with_obligations", func(t *testing.T) {
		dec, err := engine.AdmitModel(ctx, admission.ModelAdmissionInput{
			SubjectID: "model-02", ArtifactIntegrityState: admission.StateVerified,
			SBOMState: admission.StateConditional, SignatureState: admission.StateVerified,
			AttestationState: admission.StateVerified, LicenseState: admission.StateCompliant,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmitWithObligations, dec.Disposition)
		assert.NotEmpty(t, dec.Obligations)
	})
}

func TestStage2_ModelAdmission_NonAdmitting(t *testing.T) {
	base := admission.ModelAdmissionInput{
		SubjectID: "model-test", ArtifactIntegrityState: admission.StateVerified,
		SBOMState: admission.StateVerified, SignatureState: admission.StateVerified,
		AttestationState: admission.StateVerified, LicenseState: admission.StateCompliant,
	}
	denyIn, escIn, safeIn, quarIn := base, base, base, base
	denyIn.SignatureState = admission.StateViolated
	escIn.AttestationState = admission.StateUnknown
	safeIn.ArtifactIntegrityState = admission.StatePending
	quarIn.ArtifactIntegrityState = admission.StateQuarantined

	runModelCases(t, []modelCase{
		{name: "deny", input: denyIn, wantDisp: admission.DispositionDeny},
		{name: "escalate", input: escIn, wantDisp: admission.DispositionEscalate},
		{name: "safe_hold", input: safeIn, wantDisp: admission.DispositionSafeHold},
		{name: "quarantine", input: quarIn, wantDisp: admission.DispositionQuarantine},
	})
}

type deployCase struct {
	name     string
	input    admission.DeploymentAdmissionInput
	wantDisp admission.Disposition
}

func runDeployCases(t *testing.T, cases []deployCase) {
	t.Helper()
	engine, _ := newTestEngine(t)
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := engine.AdmitDeployment(ctx, tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDisp, dec.Disposition)
		})
	}
}

func TestStage3_DeploymentAdmission_AdmitAndConditional(t *testing.T) {
	engine, _ := newTestEngine(t)
	ctx := context.Background()

	t.Run("admit", func(t *testing.T) {
		dec, err := engine.AdmitDeployment(ctx, admission.DeploymentAdmissionInput{
			SubjectID: "deploy-01", RuntimeCompatibilityState: admission.StateCompliant,
			AcceleratorTopologyState: admission.StateCompliant, CapacityState: admission.StateCompliant,
			PolicyState: admission.StateCompliant, AirGapProfileState: admission.StateCompliant,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmit, dec.Disposition)
	})

	t.Run("admit_with_obligations", func(t *testing.T) {
		dec, err := engine.AdmitDeployment(ctx, admission.DeploymentAdmissionInput{
			SubjectID: "deploy-02", RuntimeCompatibilityState: admission.StateCompliant,
			AcceleratorTopologyState: admission.StateCompliant, CapacityState: admission.StateCompliant,
			PolicyState: admission.StateCompliant, AirGapProfileState: admission.StateConditional,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmitWithObligations, dec.Disposition)
		assert.NotEmpty(t, dec.Obligations)
	})
}

func TestStage3_DeploymentAdmission_NonAdmitting(t *testing.T) {
	base := admission.DeploymentAdmissionInput{
		SubjectID: "deploy-test", RuntimeCompatibilityState: admission.StateCompliant,
		AcceleratorTopologyState: admission.StateCompliant, CapacityState: admission.StateCompliant,
		PolicyState: admission.StateCompliant, AirGapProfileState: admission.StateCompliant,
	}
	denyIn, escIn, safeIn, quarIn := base, base, base, base
	denyIn.RuntimeCompatibilityState = admission.StateIncompatible
	escIn.AirGapProfileState = admission.StateFlagged
	safeIn.CapacityState = admission.StateExhausted
	quarIn.PolicyState = admission.StateQuarantined

	runDeployCases(t, []deployCase{
		{name: "deny", input: denyIn, wantDisp: admission.DispositionDeny},
		{name: "escalate", input: escIn, wantDisp: admission.DispositionEscalate},
		{name: "safe_hold", input: safeIn, wantDisp: admission.DispositionSafeHold},
		{name: "quarantine", input: quarIn, wantDisp: admission.DispositionQuarantine},
	})
}

type reqCase struct {
	name     string
	input    admission.RequestAdmissionInput
	wantDisp admission.Disposition
}

func runReqCases(t *testing.T, cases []reqCase) {
	t.Helper()
	engine, _ := newTestEngine(t)
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := engine.AdmitRequest(ctx, tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDisp, dec.Disposition)
		})
	}
}

func TestStage4_RequestAdmission_AdmitAndConditional(t *testing.T) {
	engine, _ := newTestEngine(t)
	ctx := context.Background()

	t.Run("admit", func(t *testing.T) {
		dec, err := engine.AdmitRequest(ctx, admission.RequestAdmissionInput{
			SubjectID: "req-01", IdentityTenantState: admission.StateVerified,
			CapabilityLeaseState: admission.StateVerified, QuotaBudgetState: admission.StateCompliant,
			RiskState: admission.StateCompliant,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmit, dec.Disposition)
	})

	t.Run("admit_with_obligations", func(t *testing.T) {
		dec, err := engine.AdmitRequest(ctx, admission.RequestAdmissionInput{
			SubjectID: "req-02", IdentityTenantState: admission.StateVerified,
			CapabilityLeaseState: admission.StateVerified, QuotaBudgetState: admission.StateCompliant,
			RiskState: admission.StateConditional,
		})
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmitWithObligations, dec.Disposition)
		assert.NotEmpty(t, dec.Obligations)
	})
}

func TestStage4_RequestAdmission_NonAdmitting(t *testing.T) {
	base := admission.RequestAdmissionInput{
		SubjectID: "req-test", IdentityTenantState: admission.StateVerified,
		CapabilityLeaseState: admission.StateVerified, QuotaBudgetState: admission.StateCompliant,
		RiskState: admission.StateCompliant,
	}
	denyIn, escIn, safeIn, quarIn := base, base, base, base
	denyIn.IdentityTenantState = admission.StateViolated
	escIn.RiskState = admission.StateFlagged
	safeIn.QuotaBudgetState = admission.StateExhausted
	quarIn.RiskState = admission.StateQuarantined

	runReqCases(t, []reqCase{
		{name: "deny", input: denyIn, wantDisp: admission.DispositionDeny},
		{name: "escalate", input: escIn, wantDisp: admission.DispositionEscalate},
		{name: "safe_hold", input: safeIn, wantDisp: admission.DispositionSafeHold},
		{name: "quarantine", input: quarIn, wantDisp: admission.DispositionQuarantine},
	})
}

func TestAdmission_ObligationsAttachment(t *testing.T) {
	engine, _ := newTestEngine(t)
	ctx := context.Background()

	dataDec, err := engine.AdmitData(ctx, admission.DataAdmissionInput{
		SubjectID: "ds-obls", ProvenanceState: admission.StateVerified,
		LicensingState: admission.StateConditional, PrivacyState: admission.StateConditional,
		IntegrityState: admission.StateVerified,
	})
	require.NoError(t, err)
	assert.Equal(t, admission.DispositionAdmitWithObligations, dataDec.Disposition)
	require.Len(t, dataDec.Obligations, 2)
	assert.Equal(t, "data.licensing.attribution", dataDec.Obligations[0].Type)
	assert.Equal(t, "data.privacy.masking", dataDec.Obligations[1].Type)

	reqDec, err := engine.AdmitRequest(ctx, admission.RequestAdmissionInput{
		SubjectID: "req-obls", IdentityTenantState: admission.StateVerified,
		CapabilityLeaseState: admission.StateConditional, QuotaBudgetState: admission.StateConditional,
		RiskState: admission.StateConditional,
	})
	require.NoError(t, err)
	assert.Equal(t, admission.DispositionAdmitWithObligations, reqDec.Disposition)
	require.Len(t, reqDec.Obligations, 3)
}

func TestAdmission_AuditReceiptAndSignature(t *testing.T) {
	engine, pubKey := newTestEngine(t)
	ctx := context.Background()

	dec, err := engine.AdmitModel(ctx, admission.ModelAdmissionInput{
		SubjectID: "model-audit", SubjectDigest: "sha256:1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff",
		ArtifactIntegrityState: admission.StateVerified, SBOMState: admission.StateVerified,
		SignatureState: admission.StateVerified, AttestationState: admission.StateVerified,
		LicenseState: admission.StateCompliant,
	})
	require.NoError(t, err)
	receipt := dec.Receipt

	assert.NotEmpty(t, receipt.ReceiptID)
	assert.Equal(t, admission.StageModel, receipt.Stage)
	assert.Equal(t, admission.DispositionAdmit, receipt.Disposition)
	assert.True(t, strings.HasPrefix(receipt.Digest, "sha256:"))
	assert.NotEmpty(t, receipt.Signature)

	parsedTime, err := time.Parse(time.RFC3339Nano, receipt.ProducedAt)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsedTime.Location())

	err = admission.VerifyReceipt(receipt, pubKey)
	assert.NoError(t, err)

	tampered := receipt
	tampered.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	err = admission.VerifyReceipt(tampered, pubKey)
	assert.ErrorIs(t, err, admission.ErrReceiptSignature)
}

func pipelineFixtures() (
	admission.DataAdmissionInput,
	admission.ModelAdmissionInput,
	admission.DeploymentAdmissionInput,
	admission.RequestAdmissionInput,
) {
	d := admission.DataAdmissionInput{
		SubjectID: "ds-pipe", ProvenanceState: admission.StateVerified,
		LicensingState: admission.StateCompliant, PrivacyState: admission.StateCompliant,
		IntegrityState: admission.StateVerified,
	}
	m := admission.ModelAdmissionInput{
		SubjectID: "model-pipe", ArtifactIntegrityState: admission.StateVerified,
		SBOMState: admission.StateVerified, SignatureState: admission.StateVerified,
		AttestationState: admission.StateVerified, LicenseState: admission.StateCompliant,
	}
	dep := admission.DeploymentAdmissionInput{
		SubjectID: "deploy-pipe", RuntimeCompatibilityState: admission.StateCompliant,
		AcceleratorTopologyState: admission.StateCompliant, CapacityState: admission.StateCompliant,
		PolicyState: admission.StateCompliant, AirGapProfileState: admission.StateCompliant,
	}
	r := admission.RequestAdmissionInput{
		SubjectID: "req-pipe", IdentityTenantState: admission.StateVerified,
		CapabilityLeaseState: admission.StateVerified, QuotaBudgetState: admission.StateCompliant,
		RiskState: admission.StateCompliant,
	}
	return d, m, dep, r
}

func TestAdmission_PipelineEvaluation_AdmitAndObligations(t *testing.T) {
	engine, _ := newTestEngine(t)
	ctx := context.Background()
	data, model, deploy, req := pipelineFixtures()

	t.Run("full pipeline admit", func(t *testing.T) {
		res, err := engine.EvaluatePipeline(ctx, data, model, deploy, req)
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmit, res.OverallDisposition)
		assert.Len(t, res.StageDecisions, 4)
		assert.Len(t, res.AuditReceipts, 4)
	})

	t.Run("pipeline aggregates obligations", func(t *testing.T) {
		condData := data
		condData.LicensingState = admission.StateConditional
		condDeploy := deploy
		condDeploy.AirGapProfileState = admission.StateConditional
		res, err := engine.EvaluatePipeline(ctx, condData, model, condDeploy, req)
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionAdmitWithObligations, res.OverallDisposition)
		assert.Len(t, res.StageDecisions, 4)
		assert.Len(t, res.Obligations, 2)
	})
}

func TestAdmission_PipelineEvaluation_ShortCircuit(t *testing.T) {
	engine, _ := newTestEngine(t)
	ctx := context.Background()
	data, model, deploy, req := pipelineFixtures()

	t.Run("short circuit on quarantine", func(t *testing.T) {
		poisonedData := data
		poisonedData.IntegrityState = admission.StateQuarantined
		res, err := engine.EvaluatePipeline(ctx, poisonedData, model, deploy, req)
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionQuarantine, res.OverallDisposition)
		assert.Len(t, res.StageDecisions, 1)
	})

	t.Run("short circuit on deny", func(t *testing.T) {
		deniedModel := model
		deniedModel.SignatureState = admission.StateViolated
		res, err := engine.EvaluatePipeline(ctx, data, deniedModel, deploy, req)
		require.NoError(t, err)
		assert.Equal(t, admission.DispositionDeny, res.OverallDisposition)
		assert.Len(t, res.StageDecisions, 2)
	})
}

func TestAdmission_InputValidationAndContext(t *testing.T) {
	engine, _ := newTestEngine(t)

	t.Run("missing subject ID", func(t *testing.T) {
		_, err := engine.AdmitData(context.Background(), admission.DataAdmissionInput{})
		assert.ErrorIs(t, err, admission.ErrInvalidInput)
	})

	t.Run("canceled context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := engine.AdmitData(canceledCtx, admission.DataAdmissionInput{SubjectID: "sub"})
		assert.ErrorIs(t, err, admission.ErrContextCanceled)
	})
}
