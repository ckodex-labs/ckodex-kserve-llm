package aipack

import (
	"sync"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// RegistryEntry holds a cached summary of a registered AIPack artifact.
type RegistryEntry struct {
	// Ref is the canonical OCI digest reference.
	Ref string

	// Kind is the artifact kind.
	Kind v1alpha2.ArtifactKind

	// Family is the resolved artifact family.
	Family v1alpha2.ArtifactFamily

	// RiskBand is the last computed risk valence band.
	RiskBand v1alpha2.RVBand

	// DeprecationPhase is the current lifecycle phase.
	DeprecationPhase DeprecationPhase

	// Namespace is the Kubernetes namespace of the owning AIPack resource.
	Namespace string

	// Name is the Kubernetes name of the owning AIPack resource.
	Name string
}

// Registry is an in-memory artifact inventory for operator-local lookups.
// It is populated by the AIPack reconciler and queried by the composition validator
// and webhook to resolve slot compatibility checks.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*RegistryEntry // keyed by OCI digest ref
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*RegistryEntry)}
}

// Upsert registers or updates an artifact entry.
func (r *Registry) Upsert(entry *RegistryEntry) {
	r.mu.Lock()
	r.entries[entry.Ref] = entry
	r.mu.Unlock()
}

// Delete removes an artifact entry by its OCI digest ref.
func (r *Registry) Delete(ref string) {
	r.mu.Lock()
	delete(r.entries, ref)
	r.mu.Unlock()
}

// Resolve returns the registry entry for the given OCI digest ref, or nil if not found.
func (r *Registry) Resolve(ref string) *RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.entries[ref]
}

// ListByKind returns all registry entries with the given artifact kind.
func (r *Registry) ListByKind(kind v1alpha2.ArtifactKind) []*RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*RegistryEntry
	for _, e := range r.entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// ListByFamily returns all registry entries with the given artifact family.
func (r *Registry) ListByFamily(family v1alpha2.ArtifactFamily) []*RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*RegistryEntry
	for _, e := range r.entries {
		if e.Family == family {
			out = append(out, e)
		}
	}
	return out
}
