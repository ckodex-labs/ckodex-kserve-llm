/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"k8s.io/utils/ptr"
)

const (
	// labelLocalCache identifies PVCs belonging to a LocalModelCache.
	labelLocalCache = "serving.ckodex.com/local-cache"
	// labelNode identifies the node a PVC is pinned to.
	labelNode = "serving.ckodex.com/node"
	// labelModelHash identifies the content-addressable model URI hash.
	labelModelHash = "serving.ckodex.com/model-hash"
	// defaultCacheNamespace is where cache PVCs are created for cluster-scoped resources.
	defaultCacheNamespace = "default"
	// cacheWorkloadNamespaceAnnotation selects the namespace for cache PVCs and Jobs.
	cacheWorkloadNamespaceAnnotation = "serving.ckodex.com/cache-namespace"
	// warmupJobPrefix is the prefix for cache-warming Jobs.
	warmupJobPrefix = "lmc-warmup"
)

// LocalModelCacheReconciler reconciles a LocalModelCache object
type LocalModelCacheReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	APIReader client.Reader
	Defaults  operatorconfig.DefaultsConfig
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=localmodelcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *LocalModelCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileID := k8stypes.UID(uuid.New().String())
	logger := log.FromContext(ctx).WithValues("reconcileID", reconcileID, "name", req.Name, "namespace", req.Namespace)
	logger.Info("Reconciling LocalModelCache")

	lmc := &servingv1alpha2.LocalModelCache{}
	if err := r.Get(ctx, req.NamespacedName, lmc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	modelHash := ModelURIHash(lmc.Spec.SourceModelURI)

	// 1. Identify target nodes: union of NodeGroup selector and WarmNodes.
	targetNodes, err := r.resolveTargetNodes(ctx, lmc)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 2. Reconcile PVC and warm-up Job per target node.
	var nodeStatuses []servingv1alpha2.NodeCacheStatus
	readyCount := int32(0)
	now := metav1.Now()

	// Track seen nodes to identify stale entries in status later
	seenTargetNodes := make(map[string]bool)

	for _, nodeName := range targetNodes {
		seenTargetNodes[nodeName] = true
		status, err := r.reconcileNodeCache(ctx, lmc, nodeName, modelHash, now)
		if err != nil {
			logger.Error(err, "Failed to reconcile cache for node", "node", nodeName)
			r.Recorder.Eventf(lmc, corev1.EventTypeWarning, "CacheFailed",
				"Failed to reconcile cache on node %s: %v", nodeName, err)
			continue
		}
		nodeStatuses = append(nodeStatuses, status)
		if status.Phase == "Ready" {
			readyCount++
		}
	}

	// 2b. Stale Node Status Cleanup: Remove entries from status for nodes no longer in targetNodes
	// This handles nodes being removed from warmNodes or the nodeGroup selector.
	finalNodeStatuses := []servingv1alpha2.NodeCacheStatus{}
	for _, prev := range lmc.Status.NodeStatuses {
		if seenTargetNodes[prev.NodeName] {
			// Find the entry in the newly computed nodeStatuses
			found := false
			for _, current := range nodeStatuses {
				if current.NodeName == prev.NodeName {
					finalNodeStatuses = append(finalNodeStatuses, current)
					found = true
					break
				}
			}
			if !found {
				// This shouldn't happen if the loop above covered all targetNodes,
				// but keep as fallback.
				finalNodeStatuses = append(finalNodeStatuses, prev)
			}
		} else {
			logger.Info("Cleanup: Removing stale node status", "node", prev.NodeName)
		}
	}
	// Add new nodes that weren't in prev status
	for _, current := range nodeStatuses {
		alreadyAdded := false
		for _, added := range finalNodeStatuses {
			if added.NodeName == current.NodeName {
				alreadyAdded = true
				break
			}
		}
		if !alreadyAdded {
			finalNodeStatuses = append(finalNodeStatuses, current)
		}
	}

	// 3. LRU eviction: if maxCacheSize is set, evict oldest entries.
	if err := r.evictLRU(ctx, lmc, finalNodeStatuses); err != nil {
		logger.Error(err, "LRU eviction failed")
		r.Recorder.Event(lmc, corev1.EventTypeWarning, "EvictionFailed", err.Error())
	}

	// 4. Build CachedModels status and size aggregation.
	cachedModels, totalSize := r.buildCachedModelsStatus(lmc, finalNodeStatuses)

	// 5. Update status.
	lmc.Status.NodeStatuses = finalNodeStatuses
	lmc.Status.CachedNodes = readyCount
	lmc.Status.CachedModels = cachedModels
	lmc.Status.TotalCacheSize = totalSize.String()

	if maxQ, ok := lmc.Spec.MaxCacheSizeQuantity(); ok {
		avail := maxQ.DeepCopy()
		avail.Sub(totalSize)
		if avail.Cmp(resource.MustParse("0")) < 0 {
			avail = resource.MustParse("0")
		}
		lmc.Status.AvailableSpace = avail.String()
	} else {
		lmc.Status.AvailableSpace = ""
	}

	if err := r.Status().Update(ctx, lmc); err != nil {
		return ctrl.Result{}, err
	}

	// Re-queue periodically to check Job completion and staleness.
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// IsNamespaceAllowed returns true when the given namespace is permitted to
// reference this LocalModelCache. An empty AllowedNamespaces list means
// open cluster-wide access (backward compatible default).
func IsNamespaceAllowed(lmc *servingv1alpha2.LocalModelCache, namespace string) bool {
	if len(lmc.Spec.AllowedNamespaces) == 0 {
		return true // unrestricted
	}
	for _, ns := range lmc.Spec.AllowedNamespaces {
		if ns == namespace || ns == "*" {
			return true
		}
	}
	return false
}

// resolveTargetNodes returns a deduplicated list of node names from
// both the NodeGroup label selector and the WarmNodes list.
func (r *LocalModelCacheReconciler) resolveTargetNodes(ctx context.Context, lmc *servingv1alpha2.LocalModelCache) ([]string, error) {
	reader := r.APIReader
	if reader == nil {
		reader = r.Client // Fallback for unit tests where APIReader isn't set
	}
	seen := map[string]bool{}
	var result []string

	// Nodes from label selector.
	if lmc.Spec.NodeGroup != nil && lmc.Spec.NodeGroup.LabelSelector != nil {
		nodes := &corev1.NodeList{}
		ls, _ := metav1.LabelSelectorAsSelector(lmc.Spec.NodeGroup.LabelSelector)
		if err := reader.List(ctx, nodes, &client.ListOptions{LabelSelector: ls}); err != nil {
			return nil, fmt.Errorf("listing nodes by selector: %w", err)
		}
		for _, n := range nodes.Items {
			if !seen[n.Name] {
				seen[n.Name] = true
				result = append(result, n.Name)
			}
		}
	}

	// Nodes from warmNodes list — pre-warm even if no LLMInferenceService
	// references this model yet.
	for _, name := range lmc.Spec.WarmNodes {
		if !seen[name] {
			// Verify node exists using APIReader to bypass potential cache delays in envtest.
			node := &corev1.Node{}
			if err := reader.Get(ctx, client.ObjectKey{Name: name}, node); err != nil {
				if errors.IsNotFound(err) {
					log.FromContext(ctx).Info("WarmNode not found, skipping", "node", name)
					continue
				}
				return nil, fmt.Errorf("checking warm node %s: %w", name, err)
			}
			seen[name] = true
			result = append(result, name)
		}
	}

	// If neither selector nor warmNodes is set, select all schedulable nodes.
	if lmc.Spec.NodeGroup == nil && len(lmc.Spec.WarmNodes) == 0 {
		nodes := &corev1.NodeList{}
		if err := reader.List(ctx, nodes); err != nil {
			return nil, fmt.Errorf("listing all nodes: %w", err)
		}
		for _, n := range nodes.Items {
			if !n.Spec.Unschedulable {
				result = append(result, n.Name)
			}
		}
	}

	return result, nil
}

// ModelURIHash produces a short content-addressable hash for a model URI.
// This enables deduplication: multiple LLMInferenceService resources with the
// same model URI share one PVC per node.
func ModelURIHash(uri string) string {
	h := sha256.Sum256([]byte(uri))
	return fmt.Sprintf("%x", h[:8]) // 16-char hex
}

// PVCNameForNode returns the content-addressable PVC name for a model on a node.
func PVCNameForNode(modelHash, nodeName string) string {
	// Keep the PVC name deterministic and within the 63-char DNS label limit.
	nodeHash := fmt.Sprintf("%x", sha256.Sum256([]byte(nodeName)))[:8]
	return fmt.Sprintf("lmc-%s-%s", modelHash, nodeHash)
}

func cacheWorkloadNamespace(lmc *servingv1alpha2.LocalModelCache) string {
	if namespace := lmc.Annotations[cacheWorkloadNamespaceAnnotation]; namespace != "" {
		return namespace
	}
	return defaultCacheNamespace
}

// reconcileNodeCache ensures a PVC and warm-up Job exist for one node.
func (r *LocalModelCacheReconciler) reconcileNodeCache(
	ctx context.Context,
	lmc *servingv1alpha2.LocalModelCache,
	nodeName string,
	modelHash string,
	now metav1.Time,
) (servingv1alpha2.NodeCacheStatus, error) {
	pvcName := PVCNameForNode(modelHash, nodeName)
	targetNamespace := cacheWorkloadNamespace(lmc)

	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}

	status := servingv1alpha2.NodeCacheStatus{
		NodeName:     nodeName,
		PVCName:      pvcName,
		Phase:        "Pending",
		ModelURIHash: modelHash,
	}

	// --- Job Name ---
	// jobName is content-addressable and node-specific.
	jobName := fmt.Sprintf("%s-%s-%s", warmupJobPrefix, modelHash, fmt.Sprintf("%x", sha256.Sum256([]byte(nodeName)))[:8])

	// --- 1. PVC ---
	pvc := &corev1.PersistentVolumeClaim{}
	err := reader.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: targetNamespace}, pvc)
	if err != nil && errors.IsNotFound(err) {
		// PVC is gone! Delete the Job too so it re-warms from scratch.
		orphanJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: targetNamespace}}
		_ = r.Delete(ctx, orphanJob, client.PropagationPolicy(metav1.DeletePropagationForeground))

		desiredPVC := r.buildCachePVC(lmc, pvcName, targetNamespace, nodeName, modelHash)
		log.FromContext(ctx).Info("Creating new cache PVC", "node", nodeName, "pvc", pvcName)
		if err := ctrl.SetControllerReference(lmc, desiredPVC, r.Scheme); err != nil {
			return status, fmt.Errorf("setting owner ref on PVC: %w", err)
		}
		if err := r.Create(ctx, desiredPVC); err != nil {
			return status, fmt.Errorf("creating PVC %s: %w", pvcName, err)
		}
		status.LastTransitionTime = &now
		return status, nil // Re-reconcile to create Job fresh
	} else if err != nil {
		return status, fmt.Errorf("getting PVC %s: %w", pvcName, err)
	}

	// --- 2. Warm-up Job ---
	job := &batchv1.Job{}
	err = reader.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: jobName}, job)
	if err == nil && job.DeletionTimestamp != nil {
		// Job is being deleted. Wait for it to be gone before re-creating.
		log.FromContext(ctx).Info("Waiting for old Job deletion to complete", "job", jobName)
		return status, nil
	}
	if errors.IsNotFound(err) {
		job = r.buildWarmupJob(lmc, jobName, pvcName, targetNamespace, nodeName)
		if err := ctrl.SetControllerReference(lmc, job, r.Scheme); err != nil {
			return status, fmt.Errorf("setting owner ref on Job: %w", err)
		}
		if err := r.Create(ctx, job); err != nil {
			return status, fmt.Errorf("creating warm-up Job %s: %w", jobName, err)
		}
		status.Phase = "Downloading"
		status.LastTransitionTime = &now
		r.Recorder.Eventf(lmc, corev1.EventTypeNormal, "WarmupStarted",
			"Started warm-up Job %s on node %s", jobName, nodeName)
	} else if err != nil {
		return status, fmt.Errorf("getting Job %s: %w", jobName, err)
	} else {
		// Evaluate Job status.
		status.Phase = jobPhase(job)
		if status.Phase == "Ready" {
			// Record metrics on transition to Ready
			if job.Status.CompletionTime != nil && job.Status.StartTime != nil {
				duration := job.Status.CompletionTime.Sub(job.Status.StartTime.Time).Seconds()
				observability.LMCDownloadDuration.WithLabelValues(lmc.Spec.SourceModelURI, nodeName).Observe(duration)
				observability.LMCWarmingAttempts.WithLabelValues(lmc.Spec.SourceModelURI, nodeName, "success").Inc()
			}

			// Preserve existing LastUsed if possible, otherwise use now.
			status.LastUsed = &now
			for _, prev := range lmc.Status.NodeStatuses {
				if prev.NodeName == nodeName && prev.Phase == "Ready" && prev.LastUsed != nil {
					status.LastUsed = prev.LastUsed
					break
				}
			}
			q := lmc.Spec.ModelSizeQuantity()
			status.SizeBytes = q.Value()
			observability.LMCCacheSize.WithLabelValues(lmc.Spec.SourceModelURI, nodeName).Set(float64(status.SizeBytes))
		} else if status.Phase == "Failed" {
			// Record metrics on transition to Failed
			observability.LMCWarmingAttempts.WithLabelValues(lmc.Spec.SourceModelURI, nodeName, "failed").Inc()

			// Self-Healing
			// Self-Healing: If Job failed, check failure time.
			// Recover by deleting the Job if it's been failed for > 5 minutes,
			// allowing it to be recreated on next reconcile.
			var failedTime *metav1.Time
			for _, cond := range job.Status.Conditions {
				if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
					failedTime = &cond.LastTransitionTime
					break
				}
			}
			if failedTime != nil && time.Since(failedTime.Time) > 5*time.Minute {
				log.FromContext(ctx).Info("Self-Healing: Deleting failed Job for re-warm", "job", jobName, "node", nodeName)
				r.Recorder.Eventf(lmc, corev1.EventTypeNormal, "CacheSelfHealing", "Deleting failed Job %s for re-warm", jobName)
				_ = r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
				status.Phase = "Pending" // Will be recreated next loop
			}
		}
		status.LastTransitionTime = &now
	}

	return status, nil
}

// buildCachePVC constructs a PVC with node affinity for cache pinning.
func (r *LocalModelCacheReconciler) buildCachePVC(
	lmc *servingv1alpha2.LocalModelCache,
	pvcName, namespace, nodeName, modelHash string,
) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				labelLocalCache: lmc.Name,
				labelNode:       nodeName,
				labelModelHash:  modelHash,
			},
			Annotations: map[string]string{
				"serving.ckodex.com/model-uri": lmc.Spec.SourceModelURI,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: lmc.Spec.ModelSizeQuantity(),
				},
			},
		},
	}

	if lmc.Spec.StorageClassName != nil {
		pvc.Spec.StorageClassName = lmc.Spec.StorageClassName
	}

	return pvc
}

// buildWarmupJob constructs a Job that downloads model data into a cache PVC.
// Using a Job instead of a bare Pod provides retry semantics and backoff.
func (r *LocalModelCacheReconciler) buildWarmupJob(
	lmc *servingv1alpha2.LocalModelCache,
	jobName, pvcName, namespace, nodeName string,
) *batchv1.Job {
	backoffLimit := int32(3)
	storageImage := r.Defaults.CustomStorageInitializerImage
	if storageImage == "" {
		storageImage = CKodexStorageInitializerImage
	}
	cpuRequest := r.Defaults.CacheCPURequest
	if cpuRequest == "" {
		cpuRequest = DefaultCacheCPURequest
	}
	memoryRequest := r.Defaults.CacheMemoryRequest
	if memoryRequest == "" {
		memoryRequest = DefaultCacheMemoryRequest
	}
	grace := r.Defaults.ASRTerminationGracePeriodSeconds
	if grace == 0 {
		grace = ASRTerminationGracePeriod
	}
	container := corev1.Container{
		Name:            "warmup",
		Image:           storageImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{lmc.Spec.SourceModelURI, ModelMountPath},
		Env: append(lmc.Spec.Env, []corev1.EnvVar{
			{Name: "S3_ENDPOINT", Value: SeaweedFSFilerS3Endpoint},
			{Name: "AWS_ENDPOINT_URL", Value: SeaweedFSFilerS3Endpoint},
			{Name: "AWS_NO_SIGN_REQUEST", Value: "yes"},
			{Name: "S3_USE_HTTPS", Value: "false"},
			{Name: "S3_USE_PATH_STYLE", Value: "true"},
		}...),
		VolumeMounts: []corev1.VolumeMount{
			{Name: "cache", MountPath: ModelMountPath},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: ptr.To(int64(0)),
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpuRequest),
				corev1.ResourceMemory: resource.MustParse(memoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpuRequest),
				corev1.ResourceMemory: resource.MustParse(memoryRequest),
			},
		},
	}

	podSpec := corev1.PodSpec{
		NodeSelector: map[string]string{
			"kubernetes.io/hostname": nodeName,
		},
		RestartPolicy:                 corev1.RestartPolicyOnFailure,
		TerminationGracePeriodSeconds: ptr.To(grace),
		Containers:                    []corev1.Container{container},
		Volumes: []corev1.Volume{
			{
				Name: "cache",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			},
		},
	}

	if lmc.Spec.Storage != nil {
		if lmc.Spec.Storage.SecretName != "" {
			podSpec.Containers[0].EnvFrom = append(podSpec.Containers[0].EnvFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: lmc.Spec.Storage.SecretName},
				},
			})
		}
		if lmc.Spec.Storage.ServiceAccountName != "" {
			podSpec.ServiceAccountName = lmc.Spec.Storage.ServiceAccountName
		}
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				labelLocalCache: lmc.Name,
				labelNode:       nodeName,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: podSpec,
			},
		},
	}
}

// jobPhase maps a Job's condition set to a LocalModelCache phase string.
func jobPhase(job *batchv1.Job) string {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return "Ready"
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return "Failed"
		}
	}
	if job.Status.Active > 0 {
		return "Downloading"
	}
	return "Pending"
}

// evictLRU deletes the least-recently-used cache PVCs when total size exceeds
// the configured maxCacheSize. Eviction targets PVCs owned by this controller.
func (r *LocalModelCacheReconciler) evictLRU(
	ctx context.Context,
	lmc *servingv1alpha2.LocalModelCache,
	nodeStatuses []servingv1alpha2.NodeCacheStatus,
) error {
	maxQ, ok := lmc.Spec.MaxCacheSizeQuantity()
	if !ok {
		return nil // No cap configured.
	}

	// Sort by last-used ascending (oldest first) for LRU.
	type entry struct {
		idx  int
		last time.Time
		size int64
	}
	var entries []entry
	for i, ns := range nodeStatuses {
		if ns.Phase != "Ready" {
			continue
		}
		t := time.Time{}
		if ns.LastUsed != nil {
			t = ns.LastUsed.Time
		}
		entries = append(entries, entry{idx: i, last: t, size: ns.SizeBytes})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].last.Before(entries[j].last)
	})

	// Compute total and evict from oldest until under budget.
	total := resource.Quantity{}
	for _, e := range entries {
		total.Add(*resource.NewQuantity(e.size, resource.BinarySI))
	}

	logger := log.FromContext(ctx)
	targetNamespace := cacheWorkloadNamespace(lmc)

	for _, e := range entries {
		if total.Cmp(maxQ) <= 0 {
			break
		}
		ns := nodeStatuses[e.idx]
		logger.Info("LRU evicting cache PVC", "pvc", ns.PVCName, "node", ns.NodeName,
			"lastUsed", e.last, "size", e.size)

		// Delete PVC.
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: ns.PVCName}, pvc); err == nil {
			if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("deleting evicted PVC %s: %w", ns.PVCName, err)
			}
		}

		// Delete associated Job.
		jobName := fmt.Sprintf("%s-%s-%s", warmupJobPrefix, ns.ModelURIHash,
			fmt.Sprintf("%x", sha256.Sum256([]byte(ns.NodeName)))[:8])
		job := &batchv1.Job{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: jobName}, job); err == nil {
			propagation := metav1.DeletePropagationBackground
			if err := r.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("deleting evicted Job %s: %w", jobName, err)
			}
		}

		total.Sub(*resource.NewQuantity(e.size, resource.BinarySI))
		nodeStatuses[e.idx].Phase = "Pending"

		r.Recorder.Eventf(lmc, corev1.EventTypeNormal, "CacheEvicted",
			"LRU evicted PVC %s on node %s (size=%d)", ns.PVCName, ns.NodeName, e.size)
	}

	return nil
}

// buildCachedModelsStatus aggregates per-node statuses into a model-level view.
func (r *LocalModelCacheReconciler) buildCachedModelsStatus(
	lmc *servingv1alpha2.LocalModelCache,
	nodeStatuses []servingv1alpha2.NodeCacheStatus,
) ([]servingv1alpha2.CachedModelStatus, resource.Quantity) {
	total := resource.Quantity{}
	nodeNames := []string{}
	var latestUsed *metav1.Time
	var totalBytes int64

	for _, ns := range nodeStatuses {
		if ns.Phase != "Ready" {
			continue
		}
		nodeNames = append(nodeNames, ns.NodeName)
		totalBytes += ns.SizeBytes
		total.Add(*resource.NewQuantity(ns.SizeBytes, resource.BinarySI))
		if ns.LastUsed != nil && (latestUsed == nil || ns.LastUsed.After(latestUsed.Time)) {
			latestUsed = ns.LastUsed
		}
	}

	if len(nodeNames) == 0 {
		return nil, total
	}

	pvcName := PVCNameForNode(ModelURIHash(lmc.Spec.SourceModelURI), nodeNames[0])

	models := []servingv1alpha2.CachedModelStatus{
		{
			ModelURI:  lmc.Spec.SourceModelURI,
			NodeNames: nodeNames,
			SizeBytes: totalBytes,
			LastUsed:  latestUsed,
			PVCName:   pvcName,
		},
	}

	return models, total
}

// SetupWithManager sets up the controller with the Manager.
func (r *LocalModelCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.APIReader = mgr.GetAPIReader()
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		For(&servingv1alpha2.LocalModelCache{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.mapNodeToLMC)).
		Complete(r)
}

// mapNodeToLMC enqueues all LocalModelCache objects when a Node changes.
// This ensures that node label updates (e.g. warm-node) trigger cache assignments.
func (r *LocalModelCacheReconciler) mapNodeToLMC(ctx context.Context, obj client.Object) []reconcile.Request {
	var lmcList servingv1alpha2.LocalModelCacheList
	if err := r.List(ctx, &lmcList); err != nil {
		return nil
	}
	var requests []reconcile.Request
	for _, lmc := range lmcList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&lmc),
		})
	}
	return requests
}
