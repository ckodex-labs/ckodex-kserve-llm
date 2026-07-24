//go:build e2e

/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	operatorNamespace = "ckodex-system"
	operatorName      = "ckodex-controller-manager"
)

var operatorLabels = client.MatchingLabels{
	"app.kubernetes.io/name":      "ckodex-kserve-llm-operator",
	"app.kubernetes.io/component": "controller-manager",
}

// TestE2E_Chaos_ControllerPodRestartRecovers proves that a controller pod kill
// is bounded to the disposable cluster and that the Deployment restores service.
func TestE2E_Chaos_ControllerPodRestartRecovers(t *testing.T) {
	ctx := context.Background()
	var pods corev1.PodList
	require.NoError(t, k8sClient.List(ctx, &pods, client.InNamespace(operatorNamespace), operatorLabels))
	require.Len(t, pods.Items, 1, "the chaos target must resolve to exactly one controller pod")
	victim := pods.Items[0]
	require.NoError(t, k8sClient.Delete(ctx, &victim))

	key := types.NamespacedName{Name: operatorName, Namespace: operatorNamespace}
	require.NoError(t, wait.PollUntilContextTimeout(ctx, 2*time.Second, shortTimeout, true,
		func(ctx context.Context) (bool, error) {
			var deployment appsv1.Deployment
			if err := k8sClient.Get(ctx, key, &deployment); err != nil {
				return false, err
			}
			return deployment.Status.AvailableReplicas == 1, nil
		}), "controller Deployment must become available after the pod kill")

	require.NoError(t, wait.PollUntilContextTimeout(ctx, 2*time.Second, shortTimeout, true,
		func(ctx context.Context) (bool, error) {
			var replacementPods corev1.PodList
			if err := k8sClient.List(ctx, &replacementPods, client.InNamespace(operatorNamespace), operatorLabels); err != nil {
				return false, err
			}
			for _, pod := range replacementPods.Items {
				if pod.UID != victim.UID && isPodReady(&pod) {
					return true, nil
				}
			}
			return false, nil
		}), "a new ready controller pod must replace the deleted pod")
}

func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
