/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"fmt"
	"time"

	v2 "github.com/ckodex-labs/kserve-llm-operator/internal/protocol/v2"
)

// resolveRoute picks the request endpoint using the fastest available path.
// Warm KV-cache hits keep their routing metadata consistent with the main router.
func (p *RequestPipeline) resolveRoute(ctx context.Context, req *InferenceRequest, start time.Time) RouteResult {
	if endpoint, _, pipelined := p.pipeliner.GetPipelinedEndpoint(req.SessionID); pipelined {
		return p.warmedRouteResult(endpoint, start)
	}

	if endpoint, ok := p.prefetcher.GetWarmedEndpoint(req.SessionID); ok {
		return p.warmedRouteResult(endpoint, start)
	}

	return p.router.Route(ctx, req.SessionEndpoint, req.Candidates)
}

func (p *RequestPipeline) warmedRouteResult(endpoint string, start time.Time) RouteResult {
	conn := p.pool.Get(endpoint)
	return RouteResult{
		Endpoint:           endpoint,
		CacheHit:           true,
		EstimatedLatencyMs: conn.AvgLatencyMs.Load(),
		RoutingLatency:     time.Since(start),
	}
}

func (p *RequestPipeline) runInference(ctx context.Context, conn *EndpointConn, req *InferenceRequest, endpoint string) error {
	v2Client := v2.NewClient(
		fmt.Sprintf("http://%s", endpoint),
		v2.WithHTTPClient(conn.Client),
	)

	v2Resp, err := v2Client.Infer(ctx, req.Model, "", buildInferRequest(req))
	if err != nil {
		return err
	}

	// TODO(ckodex): expose the v2 response payload through InferenceResponse when
	// this pipeline becomes the response owner.
	_ = v2Resp
	return nil
}

func buildInferRequest(req *InferenceRequest) *v2.InferRequest {
	return &v2.InferRequest{
		ID: req.SessionID,
		Inputs: []v2.InferInput{
			{
				Name:     "prompt",
				Shape:    []int64{1},
				Datatype: v2.DatatypeBYTES,
				Data:     []string{req.Prompt},
			},
		},
	}
}

func (p *RequestPipeline) releaseRequest(endpoint string, start time.Time, conn *EndpointConn) {
	conn.ActiveRequests.Add(-1)
	p.pool.RecordLatency(endpoint, time.Since(start))
}
