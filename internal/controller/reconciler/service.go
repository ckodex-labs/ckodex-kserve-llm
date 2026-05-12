package reconciler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"k8s.io/utils/ptr"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ServiceReconciler manages the inference Service.
type ServiceReconciler struct {
	Client     client.Client
	Scheme     *runtime.Scheme
	EnableGRPC bool
}

// Reconcile creates or updates the inference Service.
func (r *ServiceReconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	_ = log.FromContext(ctx) // Pre-declared for sub-reconciler uniformity

	labels := map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}

	ports := []corev1.ServicePort{
		{
			Name:        "http-inference",
			Protocol:    corev1.ProtocolTCP,
			AppProtocol: ptr.To("http"),
			Port:        80,
			TargetPort:  intstr.FromInt32(8000),
		},
	}
	if r.EnableGRPC {
		ports = append(ports, corev1.ServicePort{
			Name:        "grpc-inference",
			Protocol:    corev1.ProtocolTCP,
			AppProtocol: ptr.To("grpc"),
			Port:        8001,
			TargetPort:  intstr.FromInt32(8001),
		})
	}

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name,
			Namespace: llmSvc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector:              labels,
			Ports:                 ports,
			Type:                  corev1.ServiceTypeClusterIP,
			InternalTrafficPolicy: ptr.To(corev1.ServiceInternalTrafficPolicyLocal),
		},
	}

	// Double-down on Pristine: Create a Headless Service for distributed inference (Phase 2)
	headless := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name + "-headless",
			Namespace: llmSvc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector:  labels,
			Ports:     ports,
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone, // Headless
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}
	if err := controllerutil.SetControllerReference(llmSvc, headless, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference (headless): %w", err)
	}

	// Reconcile ClusterIP Service
	if err := r.reconcileSingleService(ctx, llmSvc, desired); err != nil {
		return err
	}

	// Reconcile Headless Service
	if err := r.reconcileSingleService(ctx, llmSvc, headless); err != nil {
		return err
	}

	return nil
}

func (r *ServiceReconciler) reconcileSingleService(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, desired *corev1.Service) error {
	logger := log.FromContext(ctx)
	var existing corev1.Service
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating Service", "name", desired.Name)
		return r.Client.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get service %s: %w", desired.Name, err)
	}

	// Update only if ports or selector changed
	if !equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) ||
		!equality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Ports = desired.Spec.Ports
		existing.Spec.Selector = desired.Spec.Selector
		logger.Info("updating Service", "name", desired.Name)
		return r.Client.Update(ctx, &existing)
	}
	return nil
}
