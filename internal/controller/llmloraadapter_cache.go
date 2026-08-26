package controller

import (
	"context"
	"crypto/sha256"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func newLoraCache(lora *servingv1alpha2.LLMLoraAdapter) *servingv1alpha2.LocalModelCache {
	return &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:   loraCacheName(lora.Namespace, lora.Name),
			Labels: map[string]string{loraCacheManagedByLabel: loraCacheManagedByAdapter},
			Annotations: map[string]string{
				loraCacheOwnerNamespace: lora.Namespace, loraCacheOwnerName: lora.Name,
				loraCacheOwnerUID: string(lora.UID), cacheWorkloadNamespaceAnnotation: lora.Namespace,
			},
		},
		Spec: servingv1alpha2.LocalModelCacheSpec{SourceModelURI: lora.Spec.Model.URI},
	}
}

func loraCacheName(namespace, name string) string {
	sum := sha256.Sum256([]byte(namespace + "/" + name))
	return fmt.Sprintf("lora-%x", sum[:10])
}

func validateLoraCacheOwner(cache *servingv1alpha2.LocalModelCache, lora *servingv1alpha2.LLMLoraAdapter) error {
	if cache.Labels[loraCacheManagedByLabel] != loraCacheManagedByAdapter || cache.Annotations[loraCacheOwnerNamespace] != lora.Namespace || cache.Annotations[loraCacheOwnerName] != lora.Name || cache.Annotations[loraCacheOwnerUID] != string(lora.UID) {
		return fmt.Errorf("LocalModelCache %s is not owned by LLMLoraAdapter %s/%s", cache.Name, lora.Namespace, lora.Name)
	}
	if cache.Spec.SourceModelURI != lora.Spec.Model.URI {
		return fmt.Errorf("LocalModelCache %s source URI does not match LLMLoraAdapter", cache.Name)
	}
	return nil
}

func (r *LLMLoraAdapterReconciler) deleteLoraCache(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) error {
	var cache servingv1alpha2.LocalModelCache
	if err := r.Get(ctx, client.ObjectKey{Name: loraCacheName(lora.Namespace, lora.Name)}, &cache); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := validateLoraCacheOwner(&cache, lora); err != nil {
		return err
	}
	return client.IgnoreNotFound(r.Delete(ctx, &cache))
}
