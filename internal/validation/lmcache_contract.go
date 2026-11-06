package validation

import (
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// The upstream multiprocess connection ConfigMap declares kv_both. Refuse
// configurations that would claim different semantics or bypass that source.
func validateMultiprocessLMCacheContract(service *servingv1alpha2.LLMInferenceService) field.ErrorList {
	cache := service.Spec.KVCache
	if cache == nil || cache.Transfer == nil || cache.Transfer.LMCache == nil || cache.Transfer.LMCache.Mode != servingv1alpha2.LMCacheModeMultiprocess {
		return nil
	}
	transfer := cache.Transfer
	root := field.NewPath("spec", "kvCache", "transfer")
	var errs field.ErrorList
	if transfer.Connector != "lmcache" {
		errs = append(errs, field.Invalid(root.Child("connector"), transfer.Connector, "multiprocess mode requires connector=lmcache"))
	}
	if transfer.LMCache.EngineRef == nil || transfer.LMCache.EngineRef.Name == "" {
		errs = append(errs, field.Required(root.Child("lmcache", "engineRef", "name"), "multiprocess mode requires an engine reference"))
	}
	if len(transfer.ExtraConfig) != 0 {
		errs = append(errs, field.Forbidden(root.Child("extraConfig"), "multiprocess mode uses the engine connection ConfigMap and does not apply extraConfig"))
	}
	if transfer.Role != "" && transfer.Role != "kv_both" {
		errs = append(errs, field.Forbidden(root.Child("role"), "multiprocess LMCache connection uses kv_both only"))
	}
	if service.Spec.Prefill != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "prefill"), "multiprocess LMCache does not support directional prefill and decode roles"))
	}
	if service.Spec.Worker != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "worker"), "the KServe multi-node path does not configure multiprocess LMCache"))
	}
	if p := service.Spec.Parallelism; p != nil && p.Pipeline != nil && *p.Pipeline > 1 {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "pipeline"), "the KServe multi-node path does not configure multiprocess LMCache"))
	}
	for i, env := range transfer.Env {
		if env.Name == "CKODEX_LMCACHE_KV_TRANSFER_CONFIG" {
			errs = append(errs, field.Forbidden(root.Child("env").Index(i).Child("name"), "reserved for the referenced LMCache engine connection"))
		}
	}
	errs = append(errs, validateMultiprocessRuntimeOverrides(service.Spec.Template.Spec, field.NewPath("spec", "template", "spec"))...)
	if service.Spec.Prefill != nil {
		errs = append(errs, validateMultiprocessRuntimeOverrides(service.Spec.Prefill.Template.Spec, field.NewPath("spec", "prefill", "template", "spec"))...)
	}
	return errs
}

func validateMultiprocessRuntimeOverrides(spec corev1.PodSpec, root *field.Path) field.ErrorList {
	if len(spec.Containers) == 0 {
		return nil
	}
	c := spec.Containers[0]
	container := root.Child("containers").Index(0)
	var errs field.ErrorList
	for i, env := range c.Env {
		if env.Name == "CKODEX_LMCACHE_KV_TRANSFER_CONFIG" {
			errs = append(errs, field.Forbidden(container.Child("env").Index(i).Child("name"), "reserved for the referenced LMCache engine connection"))
		}
	}
	for _, args := range []struct {
		name   string
		values []string
	}{{"command", c.Command}, {"args", c.Args}} {
		for i, arg := range args.values {
			if arg == "--kv-transfer-config" || strings.HasPrefix(arg, "--kv-transfer-config=") {
				errs = append(errs, field.Forbidden(container.Child(args.name).Index(i), "multiprocess LMCache requires the referenced engine connection"))
			}
		}
	}
	return errs
}
