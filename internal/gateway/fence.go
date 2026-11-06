package gateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// FenceOwnedRoutes removes forwarding from existing owned routes. It never
// creates a route and attempts each transport even if the other fails.
func FenceOwnedRoutes(ctx context.Context, reader client.Reader, writer client.Client, service *servingv1alpha2.LLMInferenceService) error {
	if reader == nil || writer == nil || service == nil || service.UID == "" {
		return errors.New("route fencing requires an API reader, writer, and service UID")
	}
	var httpErr, grpcErr error
	var pending sync.WaitGroup
	pending.Add(2)
	go func() {
		defer pending.Done()
		fenceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		httpErr = fenceHTTP(fenceCtx, reader, writer, service)
	}()
	go func() {
		defer pending.Done()
		fenceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		grpcErr = fenceGRPC(fenceCtx, reader, writer, service)
	}()
	pending.Wait()
	return errors.Join(httpErr, grpcErr)
}

func fenceHTTP(ctx context.Context, reader client.Reader, writer client.Client, service *servingv1alpha2.LLMInferenceService) error {
	key := types.NamespacedName{Name: service.Name + "-httproute", Namespace: service.Namespace}
	var route gwapiv1.HTTPRoute
	if err := reader.Get(ctx, key, &route); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read HTTPRoute %s: %w", key, err)
	}
	if !metav1.IsControlledBy(&route, service) {
		return fmt.Errorf("HTTPRoute %s is not controlled by LLMInferenceService", key)
	}
	before := route.DeepCopy()
	blockHTTPRoute(&route)
	if apiequality.Semantic.DeepEqual(&before.Spec, &route.Spec) {
		return nil
	}
	if err := writer.Patch(ctx, &route, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("fence HTTPRoute %s: %w", key, err)
	}
	return nil
}

func fenceGRPC(ctx context.Context, reader client.Reader, writer client.Client, service *servingv1alpha2.LLMInferenceService) error {
	key := types.NamespacedName{Name: service.Name + "-grpcroute", Namespace: service.Namespace}
	var route gwapiv1.GRPCRoute
	if err := reader.Get(ctx, key, &route); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read GRPCRoute %s: %w", key, err)
	}
	if !metav1.IsControlledBy(&route, service) {
		return fmt.Errorf("GRPCRoute %s is not controlled by LLMInferenceService", key)
	}
	before := route.DeepCopy()
	blockGRPCRoute(&route)
	if apiequality.Semantic.DeepEqual(&before.Spec, &route.Spec) {
		return nil
	}
	if err := writer.Patch(ctx, &route, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("fence GRPCRoute %s: %w", key, err)
	}
	return nil
}
