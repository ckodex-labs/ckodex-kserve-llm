/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"time"
)

// StartMaintenance runs periodic pool cleanup in the background.
func (p *RequestPipeline) StartMaintenance(ctx context.Context) {
	go p.maintainConnectionPool(ctx)
	go p.maintainCoalescer(ctx)

	<-ctx.Done()
}

func (p *RequestPipeline) maintainConnectionPool(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pool.EvictIdle(5 * time.Minute)
		}
	}
}

func (p *RequestPipeline) maintainCoalescer(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			batches := p.coalescer.Flush()
			for key, waiters := range batches {
				_ = key
				result := CoalescedResult{Data: nil, Error: nil}
				for _, ch := range waiters {
					ch <- result
					close(ch)
				}
			}
		}
	}
}
