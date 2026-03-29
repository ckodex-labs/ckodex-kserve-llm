/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/baggage"
)

// Baggage key constants — used by all layers (controller, proxy, sidecar).
// Keys follow the W3C Baggage spec: ASCII printable, no whitespace or separators.
const (
	BaggageTenantID   = "ckodex.tenant_id"
	BaggageModelName  = "ckodex.model"
	BaggageCostCenter = "ckodex.cost_center"
	BaggageNamespace  = "ckodex.namespace"
	BaggageSessionID  = "ckodex.session_id"
)

// TenantContext holds the chargeback context extracted from or to be injected
// into OTel baggage. All fields are safe to log (no secrets, no PII).
type TenantContext struct {
	TenantID   string
	ModelName  string
	CostCenter string
	Namespace  string
	SessionID  string
}

// InjectTenantBaggage injects tenant chargeback context into the OTel baggage
// carried on ctx. The updated context must be used for all downstream calls.
// Empty fields are skipped — only non-empty values become baggage members.
func InjectTenantBaggage(ctx context.Context, tc TenantContext) (context.Context, error) {
	bag := baggage.FromContext(ctx)

	pairs := []struct{ key, val string }{
		{BaggageTenantID, tc.TenantID},
		{BaggageModelName, tc.ModelName},
		{BaggageCostCenter, tc.CostCenter},
		{BaggageNamespace, tc.Namespace},
		{BaggageSessionID, tc.SessionID},
	}

	for _, p := range pairs {
		if p.val == "" {
			continue
		}
		m, err := baggage.NewMember(p.key, p.val)
		if err != nil {
			return ctx, fmt.Errorf("invalid baggage member %q=%q: %w", p.key, p.val, err)
		}
		bag, err = bag.SetMember(m)
		if err != nil {
			return ctx, fmt.Errorf("set baggage member %q: %w", p.key, err)
		}
	}

	return baggage.ContextWithBaggage(ctx, bag), nil
}

// ExtractTenantBaggage reads the tenant chargeback context from OTel baggage.
// Returns a zero-value TenantContext if no baggage is present — callers must
// treat absent fields as unknown, not as errors.
func ExtractTenantBaggage(ctx context.Context) TenantContext {
	bag := baggage.FromContext(ctx)
	return TenantContext{
		TenantID:   bag.Member(BaggageTenantID).Value(),
		ModelName:  bag.Member(BaggageModelName).Value(),
		CostCenter: bag.Member(BaggageCostCenter).Value(),
		Namespace:  bag.Member(BaggageNamespace).Value(),
		SessionID:  bag.Member(BaggageSessionID).Value(),
	}
}
