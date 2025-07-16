package config

import (
	"time"
	"fmt"
)

// GetProviderTimeout returns the timeout for a given provider, falling back to global if not set or invalid.
func GetProviderTimeout(providerName string, cfg *Config) time.Duration {
	for _, endpoint := range cfg.ProviderEndpoints {
		if endpoint.Name == providerName && endpoint.Timeout != "" {
			d, err := time.ParseDuration(endpoint.Timeout)
			if err == nil {
				return d
			}
		}
	}
	// fallback to global timeout
	d, err := time.ParseDuration(cfg.ProviderTimeout)
	if err != nil {
		return 100 * time.Millisecond // default
	}
	return d
}

// Example usage
func ConnectToProvider(providerName string, cfg *Config) {
	timeout := GetProviderTimeout(providerName, cfg)
	fmt.Printf("Connecting to %s with timeout %v\n", providerName, timeout)
	// ... use timeout in your HTTP client or connection logic
}
