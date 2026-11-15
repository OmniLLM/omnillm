package routes

import "sync"

type SecurityOptions struct {
	ShowToken           bool
	EnableConfigEdit    bool
	AllowLocalEndpoints bool
}

var (
	securityOptionsMu sync.RWMutex
	securityOptions   SecurityOptions
)

func ConfigureSecurityOptions(options SecurityOptions) {
	securityOptionsMu.Lock()
	securityOptions = options
	securityOptionsMu.Unlock()
}

func getSecurityOptions() SecurityOptions {
	securityOptionsMu.RLock()
	defer securityOptionsMu.RUnlock()
	return securityOptions
}
