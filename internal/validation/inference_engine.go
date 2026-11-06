/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package validation contains policy shared by admission and reconciliation.
package validation

import (
	"fmt"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	runtimeregistry "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/registry"
	vllmruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/vllm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	EngineVLLM       = runtimeregistry.DefaultEngine
	EngineSGLang     = runtimeregistry.SGLangEngine
	EngineTensorFold = runtimeregistry.TensorFoldEngine
)

// ValidateInferenceEngine rejects engines without an implemented runtime path.
func ValidateInferenceEngine(engine string) error {
	if _, ok := runtimeregistry.Resolve(engine); ok {
		return nil
	}
	return fmt.Errorf("unsupported inference engine %q: admitted engines are %s", engine, quotedEngineNames())
}

// ValidateInferenceRuntime applies the registered engine's shared validation
// rules before admission or reconciliation renders a workload.
func ValidateInferenceRuntime(service *servingv1alpha2.LLMInferenceService) field.ErrorList {
	if service == nil {
		return field.ErrorList{field.Required(field.NewPath("spec"), "service is required")}
	}
	adapter, ok := runtimeregistry.Resolve(service.Spec.Engine)
	if !ok {
		return nil
	}
	errs := field.ErrorList{}
	if qualification, requested := service.Annotations[inferenceruntime.RuntimeQualificationAnnotation]; requested {
		path := field.NewPath("metadata", "annotations").Key(inferenceruntime.RuntimeQualificationAnnotation)
		if adapter.Name() != EngineVLLM {
			errs = append(errs, field.Forbidden(path, "runtime qualification is only supported for vLLM"))
		} else if qualification != vllmruntime.RuntimeAcceptanceVersion {
			errs = append(errs, field.NotSupported(path, qualification, []string{vllmruntime.RuntimeAcceptanceVersion}))
		} else {
			image := adapter.Image()
			if !image.Valid() {
				errs = append(errs, field.InternalError(path, fmt.Errorf("vLLM adapter image contract is invalid")))
			} else {
				imagePath := field.NewPath("spec", "template", "spec", "containers").Index(0).Child("image")
				if len(service.Spec.Template.Spec.Containers) == 0 {
					errs = append(errs, field.Required(imagePath, "the runtime acceptance path requires an explicit digest-pinned image"))
				} else {
					actual := service.Spec.Template.Spec.Containers[0].Image
					expected := image.Reference()
					if actual != expected && !strings.HasSuffix(actual, "/"+expected) {
						errs = append(errs, field.Invalid(imagePath, actual, "image must match the digest-pinned vLLM v0.33.0 contract or its registry mirror"))
					}
					if accessErrs := validateQualificationAccess(service.Spec.Template.Spec.Containers[0]); len(accessErrs) > 0 {
						errs = append(errs, accessErrs...)
					}
				}
				if service.Spec.Router.Gateway.ExistingRef == nil || service.Spec.Router.Gateway.Managed != nil {
					errs = append(errs, field.Forbidden(
						field.NewPath("spec", "router", "gateway"),
						"vLLM qualification requires an existing Gateway; managed plaintext listeners are not accepted",
					))
				}
			}
		}
	}
	return append(errs, adapter.Validate(service)...)
}

func validateQualificationAccess(container corev1.Container) field.ErrorList {
	containerPath := field.NewPath("spec", "template", "spec", "containers").Index(0)
	if len(container.EnvFrom) != 0 {
		return field.ErrorList{field.Forbidden(containerPath.Child("envFrom"), "the runtime acceptance path requires explicit environment entries so security-sensitive settings cannot be hidden in imported environment variables")}
	}
	for index, argument := range container.Args {
		option := normalizeRuntimeFlag(argument)
		jsonFlag, _, _ := strings.Cut(option, ".")
		if matchesRuntimeFlag(option, "--api-key") {
			return field.ErrorList{field.Forbidden(containerPath.Child("args").Index(index), "API keys must not be stored in container arguments; use a Kubernetes Secret-backed environment variable")}
		}
		if matchesRuntimeFlag(option, "--config") || matchesRuntimeFlag(jsonFlag, "--json-arg") {
			return field.ErrorList{field.Forbidden(containerPath.Child("args").Index(index), "acceptance workloads cannot load external or JSON CLI configuration that could override the Secret-backed API key")}
		}
		if matchesRuntimeFlag(option, "--trust-remote-code") {
			return field.ErrorList{field.Forbidden(containerPath.Child("args").Index(index), "remote model code must remain disabled for the runtime acceptance path")}
		}
		if matchesRuntimeFlag(option, "--enable-fastapi-docs") || matchesRuntimeFlag(option, "--no-disable-fastapi-docs") {
			return field.ErrorList{field.Forbidden(containerPath.Child("args").Index(index), "FastAPI documentation endpoints must remain disabled for the runtime acceptance path")}
		}
	}
	path := containerPath.Child("env")
	secretRefs := 0
	for _, value := range container.Env {
		if value.Name != "VLLM_API_KEY" {
			continue
		}
		if value.Value != "" || value.ValueFrom == nil || value.ValueFrom.SecretKeyRef == nil || value.ValueFrom.SecretKeyRef.Name == "" || value.ValueFrom.SecretKeyRef.Key == "" {
			return field.ErrorList{field.Forbidden(path, "VLLM_API_KEY must come from a named Kubernetes Secret key reference")}
		}
		if value.ValueFrom.SecretKeyRef.Optional != nil && *value.ValueFrom.SecretKeyRef.Optional {
			return field.ErrorList{field.Forbidden(path, "VLLM_API_KEY Secret references must be required")}
		}
		secretRefs++
	}
	if secretRefs != 1 {
		return field.ErrorList{field.Required(path, "the runtime acceptance path requires exactly one Secret-backed VLLM_API_KEY")}
	}
	return nil
}

func normalizeRuntimeFlag(argument string) string {
	if !strings.HasPrefix(argument, "--") {
		return argument
	}
	name, value, hasValue := strings.Cut(argument, "=")
	name = strings.ReplaceAll(name, "_", "-")
	if !hasValue {
		return name
	}
	return name + "=" + value
}

func matchesRuntimeFlag(argument, flag string) bool {
	name, _, _ := strings.Cut(normalizeRuntimeFlag(argument), "=")
	return name == flag || (len(name) < len(flag) && strings.HasPrefix(flag, name))
}

// AdmittedInferenceEngines returns the engine values accepted by admission.
func AdmittedInferenceEngines() []string {
	return runtimeregistry.Names()
}

func quotedEngineNames() string {
	names := AdmittedInferenceEngines()
	for index := range names {
		names[index] = fmt.Sprintf("%q", names[index])
	}
	return strings.Join(names, ", ")
}
