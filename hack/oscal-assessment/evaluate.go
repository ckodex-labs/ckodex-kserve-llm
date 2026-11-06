package main

import (
	"fmt"
	"slices"
)

type metadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type object struct {
	Metadata metadata `json:"metadata"`
	Status   struct {
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"conditions"`
	} `json:"status"`
	Data map[string]string `json:"data"`
}

type input struct {
	Services []object        `json:"llmServices"`
	Policies []object        `json:"networkPolicies"`
	Entries  []object        `json:"spireEntries"`
	Identity identityContext `json:"-"`
}

type result struct {
	Control string `json:"control"`
	Passed  bool   `json:"passed"`
	Reason  string `json:"reason"`
}

func evaluate(in input, control string) []result {
	controls := []string{"ac-4", "au-2", "si-7", "si-4", "sr-2", "ia-9"}
	results := make([]result, 0, len(controls))
	for _, c := range controls {
		if control != "" && control != c {
			continue
		}
		passed, reason := evaluateControl(in, c)
		results = append(results, result{Control: c, Passed: passed, Reason: reason})
	}
	return results
}

func evaluateControl(in input, control string) (bool, string) {
	if control == "ia-9" {
		if err := in.Identity.validate(); err != nil {
			return false, err.Error()
		}
		passed, reason := evaluateServices(in, control)
		return passed, reason + "; " + in.Identity.description()
	}
	return evaluateServices(in, control)
}

func evaluateServices(in input, control string) (bool, string) {
	if len(in.Services) == 0 {
		return false, "no LLMInferenceService resources"
	}
	for _, service := range in.Services {
		if service.Metadata.Namespace == "" || service.Metadata.Name == "" {
			return false, "LLMInferenceService is missing name or namespace"
		}
		if !validService(in, service, control) {
			if control == "ac-4" {
				return false, fmt.Sprintf("no NetworkPolicy found in namespace %s for %s; policy selection and enforcement not assessed", service.Metadata.Namespace, service.Metadata.Name)
			}
			return false, fmt.Sprintf("%s/%s failed the %s resource check", service.Metadata.Namespace, service.Metadata.Name, control)
		}
	}
	if control == "ac-4" {
		return true, fmt.Sprintf("NetworkPolicy present in the namespace of %d service(s); policy selection and enforcement not assessed", len(in.Services))
	}
	return true, fmt.Sprintf("%d service(s) passed the %s resource check", len(in.Services), control)
}

func validService(in input, service object, control string) bool {
	switch control {
	case "ac-4":
		for _, policy := range in.Policies {
			if policy.Metadata.Name != "" && policy.Metadata.Namespace == service.Metadata.Namespace {
				return true
			}
		}
		return false
	case "au-2":
		return condition(service, "Compliance-AU-2", "AuditPersistent")
	case "si-4":
		return condition(service, "Compliance-SI-4", "OISSignalsActive")
	// The old shared supply-chain policy required both conditions for either control.
	case "si-7", "sr-2":
		return condition(service, "Compliance-SI-7", "IntegrityVerified") && condition(service, "Compliance-SR-2", "ProvenanceVerified")
	case "ia-9":
		return validRegistration(in.Entries, service, in.Identity.selectorNamespace(service.Metadata.Namespace))
	default:
		return false
	}
}

func condition(service object, kind, reason string) bool {
	return unambiguousCondition(service, kind, reason)
}

func validRegistration(entries []object, service object, physicalNamespace string) bool {
	var matches []object
	for _, entry := range entries {
		m := entry.Metadata
		if m.Namespace == "spire" && m.Labels["spire.ckodex.com/registration-entry"] == "true" && m.Annotations["ckodex.com/source-namespace"] == service.Metadata.Namespace && m.Annotations["ckodex.com/source-service"] == service.Metadata.Name {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 {
		return false
	}
	var registration struct {
		ID        string   `json:"spiffeId"`
		Selectors []string `json:"selectors"`
		TTL       int32    `json:"ttl"`
		DNSSANs   []string `json:"dnsSans"`
	}
	if err := unmarshalUniqueJSON([]byte(matches[0].Data["entry.json"]), &registration); err != nil {
		return false
	}
	ns, name := service.Metadata.Namespace, service.Metadata.Name
	id := fmt.Sprintf("spiffe://ckodex.com/ns/%s/sa/%s/model/%s", ns, name, name)
	return registration.ID == id && matches[0].Metadata.Annotations["ckodex.com/spiffe-id"] == id && registration.TTL > 0 && registration.TTL <= 3600 && len(registration.Selectors) == 2 && slices.Contains(registration.Selectors, "k8s:ns:"+physicalNamespace) && slices.Contains(registration.Selectors, "k8s:sa:"+name) && len(registration.DNSSANs) == 1 && registration.DNSSANs[0] == name+"."+ns+".svc.cluster.local"
}
