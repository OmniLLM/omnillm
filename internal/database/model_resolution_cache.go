package database

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// ModelResolutionCache is a read-heavy in-memory cache for data queried on
// every hot-path request. Snapshots are built locally and published only after
// their database reads complete successfully.
type ModelResolutionCache struct {
	mu sync.RWMutex

	vmByName      map[string]*VirtualModelRecord
	vmUpstreams   map[string][]VirtualModelUpstreamRecord
	provInst      []ProviderInstanceRecord
	instByID      map[string]ProviderInstanceRecord
	instByLcAlias map[string][]string
	instByLcName  map[string][]string

	vmLoaded   bool
	instLoaded bool
}

type ModelStateCache struct {
	mu     sync.RWMutex
	data   map[string]map[string]bool
	loaded bool
}

var (
	globalModelResCache   = &ModelResolutionCache{}
	globalModelStateCache = &ModelStateCache{}
)

func GetModelResolutionCache() *ModelResolutionCache { return globalModelResCache }
func GetModelStateCache() *ModelStateCache           { return globalModelStateCache }

func (c *ModelResolutionCache) ensureVMLoaded() {
	c.mu.RLock()
	ok := c.vmLoaded
	c.mu.RUnlock()
	if ok {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vmLoaded {
		return
	}
	_ = c.loadVMsLocked()
}

func (c *ModelResolutionCache) loadVMsLocked() error {
	db := GetDatabase()
	rows, err := db.db.Query(`
		SELECT virtual_model_id, name, description, api_shape, lb_strategy, enabled, created_at, updated_at
		FROM virtual_models ORDER BY created_at ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	byName := make(map[string]*VirtualModelRecord)
	for rows.Next() {
		var r VirtualModelRecord
		var enabledInt int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&r.VirtualModelID, &r.Name, &r.Description, &r.APIShape, &r.LbStrategy, &enabledInt, &createdAtStr, &updatedAtStr); err != nil {
			return err
		}
		r.Enabled = enabledInt == 1
		r.CreatedAt = parseTime(createdAtStr)
		r.UpdatedAt = parseTime(updatedAtStr)
		rec := r
		byName[strings.ToLower(r.VirtualModelID)] = &rec
	}
	if err := rows.Err(); err != nil {
		return err
	}

	uRows, err := db.db.Query(`
		SELECT id, virtual_model_id, provider_id, model_id, weight, priority, created_at, updated_at
		FROM virtual_model_upstreams ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return err
	}
	defer uRows.Close()

	upstreams := make(map[string][]VirtualModelUpstreamRecord)
	for uRows.Next() {
		var u VirtualModelUpstreamRecord
		var createdAtStr, updatedAtStr string
		if err := uRows.Scan(&u.ID, &u.VirtualModelID, &u.ProviderID, &u.ModelID, &u.Weight, &u.Priority, &createdAtStr, &updatedAtStr); err != nil {
			return err
		}
		u.CreatedAt = parseTime(createdAtStr)
		u.UpdatedAt = parseTime(updatedAtStr)
		upstreams[u.VirtualModelID] = append(upstreams[u.VirtualModelID], u)
	}
	if err := uRows.Err(); err != nil {
		return err
	}

	c.vmByName = byName
	c.vmUpstreams = upstreams
	c.vmLoaded = true
	return nil
}

func (c *ModelResolutionCache) GetVirtualModel(nameOrID string) *VirtualModelRecord {
	c.ensureVMLoaded()
	c.mu.RLock()
	r := c.vmByName[strings.ToLower(nameOrID)]
	c.mu.RUnlock()
	return r
}

func (c *ModelResolutionCache) GetUpstreams(virtualModelID string) []VirtualModelUpstreamRecord {
	c.ensureVMLoaded()
	c.mu.RLock()
	u := c.vmUpstreams[virtualModelID]
	c.mu.RUnlock()
	return u
}

func (c *ModelResolutionCache) InvalidateVMs() {
	c.mu.Lock()
	c.vmLoaded = false
	c.mu.Unlock()
}

func (c *ModelResolutionCache) ensureInstLoaded() {
	c.mu.RLock()
	ok := c.instLoaded
	c.mu.RUnlock()
	if ok {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.instLoaded {
		return
	}
	_ = c.loadInstLocked()
}

func (c *ModelResolutionCache) loadInstLocked() error {
	db := GetDatabase()
	rows, err := db.db.Query(`
		SELECT instance_id, provider_id, name, subtitle, priority, activated, created_at, updated_at
		FROM provider_instances ORDER BY priority ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var records []ProviderInstanceRecord
	byID := make(map[string]ProviderInstanceRecord)
	byLcAlias := make(map[string][]string)
	byLcName := make(map[string][]string)
	for rows.Next() {
		var r ProviderInstanceRecord
		var activated int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&r.InstanceID, &r.ProviderID, &r.Name, &r.Subtitle, &r.Priority, &activated, &createdAtStr, &updatedAtStr); err != nil {
			return err
		}
		r.Activated = activated != 0
		r.CreatedAt = parseTime(createdAtStr)
		r.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, r)
		byID[r.InstanceID] = r
		aliasKey := normalizeProviderReference(r.Subtitle)
		if aliasKey != "" {
			byLcAlias[aliasKey] = append(byLcAlias[aliasKey], r.InstanceID)
		}
		nameKey := normalizeProviderReference(r.Name)
		if nameKey != "" {
			byLcName[nameKey] = append(byLcName[nameKey], r.InstanceID)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	c.provInst = records
	c.instByID = byID
	for _, ids := range byLcAlias {
		slices.Sort(ids)
	}
	for _, ids := range byLcName {
		slices.Sort(ids)
	}
	c.instByLcAlias = byLcAlias
	c.instByLcName = byLcName
	c.instLoaded = true
	return nil
}

func (c *ModelResolutionCache) GetAllProviderInstances() []ProviderInstanceRecord {
	c.ensureInstLoaded()
	c.mu.RLock()
	r := c.provInst
	c.mu.RUnlock()
	return r
}

var ErrProviderReferenceNotFound = errors.New("provider reference not found")

type AmbiguousProviderReferenceError struct {
	Reference string
	Matches   []string
}

func (e *AmbiguousProviderReferenceError) Error() string {
	return fmt.Sprintf("provider reference %q is ambiguous; matching instance IDs: %s", e.Reference, strings.Join(e.Matches, ", "))
}

func normalizeProviderReference(reference string) string {
	return strings.ToLower(strings.TrimSpace(reference))
}

// ResolveProviderReference resolves an exact instance ID, then a unique
// case-insensitive alias, then a unique case-insensitive display name.
func (c *ModelResolutionCache) ResolveProviderReference(reference string) (string, error) {
	c.ensureInstLoaded()
	c.mu.RLock()
	defer c.mu.RUnlock()
	reference = strings.TrimSpace(reference)
	if _, ok := c.instByID[reference]; ok {
		return reference, nil
	}
	key := normalizeProviderReference(reference)
	for _, matches := range [][]string{c.instByLcAlias[key], c.instByLcName[key]} {
		switch len(matches) {
		case 0:
			continue
		case 1:
			return matches[0], nil
		default:
			return "", &AmbiguousProviderReferenceError{Reference: reference, Matches: slices.Clone(matches)}
		}
	}
	return "", fmt.Errorf("%w: %q", ErrProviderReferenceNotFound, reference)
}

func (c *ModelResolutionCache) LookupProviderPrefix(prefix string) (string, bool) {
	id, err := c.ResolveProviderReference(prefix)
	return id, err == nil
}

func (c *ModelResolutionCache) ResolveProviderPrefix(prefix string) string {
	if id, ok := c.LookupProviderPrefix(prefix); ok {
		return id
	}
	return prefix
}

func (c *ModelResolutionCache) InvalidateInstances() {
	c.mu.Lock()
	c.instLoaded = false
	c.mu.Unlock()
}

func (c *ModelStateCache) ensure() {
	c.mu.RLock()
	ok := c.loaded
	c.mu.RUnlock()
	if ok {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return
	}
	_ = c.loadLocked()
}

func (c *ModelStateCache) loadLocked() error {
	rows, err := GetDatabase().db.Query(`
		SELECT instance_id, model_id, enabled FROM provider_model_states
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	data := make(map[string]map[string]bool)
	for rows.Next() {
		var instID, modelID string
		var enabledInt int
		if err := rows.Scan(&instID, &modelID, &enabledInt); err != nil {
			return err
		}
		if data[instID] == nil {
			data[instID] = make(map[string]bool)
		}
		data[instID][modelID] = enabledInt == 1
	}
	if err := rows.Err(); err != nil {
		return err
	}

	c.data = data
	c.loaded = true
	return nil
}

func (c *ModelStateCache) GetDisabledModels(instanceID string) map[string]bool {
	c.ensure()
	c.mu.RLock()
	states := c.data[instanceID]
	c.mu.RUnlock()

	disabled := make(map[string]bool)
	for modelID, enabled := range states {
		if !enabled {
			disabled[modelID] = true
		}
	}
	return disabled
}

func (c *ModelStateCache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.mu.Unlock()
}
