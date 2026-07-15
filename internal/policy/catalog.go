// Package policy owns the process-local version-to-content policy contract.
package policy

import (
	"sync"

	"github.com/zrma/sema/internal/domain"
)

type Entry struct {
	Policy      domain.MatchmakingPolicy
	Fingerprint domain.PolicyFingerprint
}

type Catalog struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

func NewCatalog() *Catalog {
	return &Catalog{entries: make(map[string]Entry)}
}

func (catalog *Catalog) Register(candidate domain.MatchmakingPolicy) (Entry, error) {
	fingerprint, err := domain.FingerprintPolicy(candidate)
	if err != nil {
		return Entry{}, err
	}
	candidate = domain.ClonePolicy(candidate)

	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	if existing, exists := catalog.entries[candidate.Version]; exists {
		if existing.Fingerprint != fingerprint {
			return Entry{}, domain.NewFailure(
				domain.FailurePolicyConflict,
				"policy version %q is already registered with different content",
				candidate.Version,
			)
		}
		return cloneEntry(existing), nil
	}
	entry := Entry{Policy: candidate, Fingerprint: fingerprint}
	catalog.entries[candidate.Version] = entry
	return cloneEntry(entry), nil
}

func (catalog *Catalog) Get(version string) (Entry, bool) {
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	entry, exists := catalog.entries[version]
	if !exists {
		return Entry{}, false
	}
	return cloneEntry(entry), true
}

func cloneEntry(entry Entry) Entry {
	entry.Policy = domain.ClonePolicy(entry.Policy)
	return entry
}
