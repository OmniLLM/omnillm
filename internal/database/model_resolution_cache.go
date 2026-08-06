package database

import (
	"strings"
	"sync"
)

// ModelResolutionCache is a read-heavy in-memory cache for the three data sets
// that are queried on every hot-path request:
//
//  1. Virtual models: map[lowercaseName → *VirtualModelRecord]
//  2. Virtual model upstreams: map[virtualModelID → []VirtualModelUpstreamRecord]
//  3. Provider instances: []ProviderInstanceRecord (sorted by priority asc)
//
// All data is populated lazily on first access and invalidated whenever the
// underlying store performs a write.  Reads are lock-free after the first load
// (swap to new map pointer under write lock; readers hold read lock for pointer
// copy only).
//
// Model state (enabled/disabled per instance) is cached separately in
// ModelStateCache, which is invalidated by SetEnabled/Delete on ModelStateStore.
type ModelResolutionCache struct {
	mu sync.RWMutex

	vmByName    map[string]*VirtualModelRecord          // key: lowercase name/id
	vmUpstreams map[string][]VirtualModelUpstreamRecord // key: virtualModelID
	provInst    []ProviderInstanceRecord
	instByID    map[string]ProviderInstanceRecord // key: instanceID
	instByLcSub map[string]string                 // key: lc subtitle → instanceID

	vmLoaded   bool
	instLoaded bool
}

// ModelStateCache caches GetAllByInstance results keyed by instanceID.
type ModelStateCache struct {
	mu     sync.RWMutex
	data   map[string]map[string]bool // instanceID → modelID → enabled
	loaded bool
}

var (
	globalModelResCache   = &ModelResolutionCache{}
	globalModelStateCache = &ModelStateCache{}
)

// GetModelResolutionCache returns the process-wide model resolution cache.
func GetModelResolutionCache() *ModelResolutionCache { return globalModelResCache }

// GetModelStateCache returns the process-wide model state cache.
func GetModelStateCache() *ModelStateCache { return globalModelStateCache }

// ─── ModelResolutionCache ─────────────────────────────────────────────────────

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
	c.loadVMsLocked()
}

func (c *ModelResolutionCache) loadVMsLocked() {
	db := GetDatabase()
	rows, err := db.db.Query(`
		SELECT virtual_model_id, name, description, api_shape, lb_strategy, enabled, created_at, updated_at
		FROM virtual_models ORDER BY created_at ASC
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	byName := make(map[string]*VirtualModelRecord)
	upstreams := make(map[string][]VirtualModelUpstreamRecord)

	var vms []VirtualModelRecord
	for rows.Next() {
		var r VirtualModelRecord
		var enabledInt int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&r.VirtualModelID, &r.Name, &r.Description, &r.APIShape, &r.LbStrategy, &enabledInt, &createdAtStr, &updatedAtStr); err != nil {
			continue
		}
		r.Enabled = enabledInt == 1
		r.CreatedAt = parseTime(createdAtStr)
		r.UpdatedAt = parseTime(updatedAtStr)
		vms = append(vms, r)
	}
	_ = rows.Err()

	for i := range vms {
		key := strings.ToLower(vms[i].VirtualModelID)
		rec := vms[i]
		byName[key] = &rec
	}

	// Load all upstreams in one query.
	uRows, err := db.db.Query(`
		SELECT id, virtual_model_id, provider_id, model_id, weight, priority, created_at, updated_at
		FROM virtual_model_upstreams ORDER BY priority ASC, id ASC
	`)
	if err == nil {
		defer uRows.Close()
		for uRows.Next() {
			var u VirtualModelUpstreamRecord
			var createdAtStr, updatedAtStr string
			if err := uRows.Scan(&u.ID, &u.VirtualModelID, &u.ProviderID, &u.ModelID, &u.Weight, &u.Priority, &createdAtStr, &updatedAtStr); err != nil {
				continue
			}
			u.CreatedAt = parseTime(createdAtStr)
			u.UpdatedAt = parseTime(updatedAtStr)
			upstreams[u.VirtualModelID] = append(upstreams[u.VirtualModelID], u)
		}
	}

	c.vmByName = byName
	c.vmUpstreams = upstreams
	c.vmLoaded = true
}

// GetVirtualModel returns the cached virtual model record for the given name/id.
func (c *ModelResolutionCache) GetVirtualModel(nameOrID string) *VirtualModelRecord {
	c.ensureVMLoaded()
	c.mu.RLock()
	r := c.vmByName[strings.ToLower(nameOrID)]
	c.mu.RUnlock()
	return r
}

// GetUpstreams returns the cached upstream list for the given virtual model ID.
func (c *ModelResolutionCache) GetUpstreams(virtualModelID string) []VirtualModelUpstreamRecord {
	c.ensureVMLoaded()
	c.mu.RLock()
	u := c.vmUpstreams[virtualModelID]
	c.mu.RUnlock()
	return u
}

// InvalidateVMs drops the virtual model + upstream cache. The next read reloads from DB.
func (c *ModelResolutionCache) InvalidateVMs() {
	c.mu.Lock()
	c.vmLoaded = false
	c.vmByName = nil
	c.vmUpstreams = nil
	c.mu.Unlock()
}

// ─── Provider instance cache ─────────────────────────────────────────────────

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
	c.loadInstLocked()
}

func (c *ModelResolutionCache) loadInstLocked() {
	db := GetDatabase()
	rows, err := db.db.Query(`
		SELECT instance_id, provider_id, name, subtitle, priority, activated, created_at, updated_at
		FROM provider_instances ORDER BY priority ASC
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	var records []ProviderInstanceRecord
	byID := make(map[string]ProviderInstanceRecord)
	byLcSub := make(map[string]string)

	for rows.Next() {
		var r ProviderInstanceRecord
		var activated int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&r.InstanceID, &r.ProviderID, &r.Name, &r.Subtitle, &r.Priority, &activated, &createdAtStr, &updatedAtStr); err != nil {
			continue
		}
		r.Activated = activated != 0
		r.CreatedAt = parseTime(createdAtStr)
		r.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, r)
		byID[r.InstanceID] = r
		lcs := strings.ToLower(r.Subtitle)
		if lcs != "" {
			if _, exists := byLcSub[lcs]; !exists {
				byLcSub[lcs] = r.InstanceID
			}
		}
	}

	c.provInst = records
	c.instByID = byID
	c.instByLcSub = byLcSub
	c.instLoaded = true
}

// GetAllProviderInstances returns the cached, priority-sorted provider instance list.
func (c *ModelResolutionCache) GetAllProviderInstances() []ProviderInstanceRecord {
	c.ensureInstLoaded()
	c.mu.RLock()
	r := c.provInst
	c.mu.RUnlock()
	return r
}

// LookupProviderPrefix maps a known prefix to an instance ID using the cache.
func (c *ModelResolutionCache) LookupProviderPrefix(prefix string) (string, bool) {
	c.ensureInstLoaded()
	c.mu.RLock()
	byID := c.instByID
	byLcSub := c.instByLcSub
	c.mu.RUnlock()

	if _, ok := byID[prefix]; ok {
		return prefix, true
	}
	id, ok := byLcSub[strings.ToLower(prefix)]
	return id, ok
}

// ResolveProviderPrefix maps a prefix to an instance ID using the cache.
func (c *ModelResolutionCache) ResolveProviderPrefix(prefix string) string {
	if id, ok := c.LookupProviderPrefix(prefix); ok {
		return id
	}
	return prefix
}

// InvalidateInstances drops the provider instance cache.
func (c *ModelResolutionCache) InvalidateInstances() {
	c.mu.Lock()
	c.instLoaded = false
	c.provInst = nil
	c.instByID = nil
	c.instByLcSub = nil
	c.mu.Unlock()
}

// ─── ModelStateCache ─────────────────────────────────────────────────────────

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

	db := GetDatabase()
	rows, err := db.db.Query(`
		SELECT instance_id, model_id, enabled FROM provider_model_states
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	data := make(map[string]map[string]bool)
	for rows.Next() {
		var instID, modelID string
		var enabledInt int
		if err := rows.Scan(&instID, &modelID, &enabledInt); err != nil {
			continue
		}
		if data[instID] == nil {
			data[instID] = make(map[string]bool)
		}
		data[instID][modelID] = enabledInt == 1
	}

	c.data = data
	c.loaded = true
}

// GetDisabledModels returns the set of disabled model IDs for the given instance.
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

// Invalidate drops the model state cache.
func (c *ModelStateCache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.data = nil
	c.mu.Unlock()
}
