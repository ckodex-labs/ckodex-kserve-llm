package main

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation"
)

// identityContext is caller-supplied assessment context, not observed cluster state.
type identityContext struct {
	Mode          string
	HostNamespace string
}

func (c identityContext) validate() error {
	switch c.Mode {
	case "direct":
		if c.HostNamespace != "" {
			return fmt.Errorf("--host-namespace is only valid with --identity-mode=vcluster")
		}
	case "vcluster":
		if errs := validation.IsDNS1123Label(c.HostNamespace); len(errs) != 0 {
			return fmt.Errorf("--identity-mode=vcluster requires a valid --host-namespace")
		}
	default:
		return fmt.Errorf("IA-9 requires --identity-mode=direct or --identity-mode=vcluster")
	}
	return nil
}

func validateIdentityForControl(control string, c identityContext) error {
	if control == "" || control == "ia-9" {
		return c.validate()
	}
	if c.Mode != "" || c.HostNamespace != "" {
		return fmt.Errorf("identity context is only valid when evaluating IA-9")
	}
	return nil
}

func (c identityContext) selectorNamespace(logicalNamespace string) string {
	if c.Mode == "vcluster" {
		return c.HostNamespace
	}
	return logicalNamespace
}

func (c identityContext) description() string {
	if c.Mode == "vcluster" {
		return fmt.Sprintf("caller-asserted identity mode=vcluster, host namespace=%s", c.HostNamespace)
	}
	return "caller-asserted identity mode=direct"
}
