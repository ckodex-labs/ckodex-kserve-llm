package deployment

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func setEnvDefault(c *corev1.Container, name, value string) {
	if !hasEnv(c.Env, name) {
		c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
	}
}

func isMultiprocessLMCache(llmSvc *servingv1alpha2.LLMInferenceService) bool {
	return llmSvc.Spec.KVCache != nil && llmSvc.Spec.KVCache.Transfer != nil && llmSvc.Spec.KVCache.Transfer.LMCache != nil && llmSvc.Spec.KVCache.Transfer.LMCache.Mode == servingv1alpha2.LMCacheModeMultiprocess && llmSvc.Spec.KVCache.Transfer.LMCache.EngineRef != nil
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src)+4)
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func hasEnv(env []corev1.EnvVar, name string) bool {
	for _, item := range env {
		if item.Name == name {
			return true
		}
	}
	return false
}

func parseKVExtraConfig(src map[string]string) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = parseKVValue(value)
	}
	return dst
}

func parseKVValue(value string) interface{} {
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed
	}
	var structured interface{}
	if json.Unmarshal([]byte(value), &structured) == nil {
		switch structured.(type) {
		case map[string]interface{}, []interface{}:
			return structured
		}
	}
	return value
}

func deploymentStrategyForReplicas(replicas int32) appsv1.DeploymentStrategy {
	if replicas <= 1 {
		return appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	}
	return appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0}, MaxSurge: &intstr.IntOrString{Type: intstr.Int, IntVal: 1}}}
}

func isNilSPIREInjector(injector SPIREInjector) bool {
	if injector == nil {
		return true
	}
	v := reflect.ValueOf(injector)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target || strings.HasPrefix(arg, target+"=") {
			return true
		}
	}
	return false
}
