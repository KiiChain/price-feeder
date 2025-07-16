import (
    "time"
    "fmt"
    "https://github.com/KiiChain/price-feeder/config"
)

// Returns the timeout for a given provider, falling back to global if not set
func getProviderTimeout(providerName string, cfg *config.Config) time.Duration {
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
func connectToProvider(providerName string, cfg *config.Config) {
    timeout := getProviderTimeout(providerName, cfg)
    fmt.Printf("Connecting to %s with timeout %v\n", providerName, timeout)
    // ... use timeout in your HTTP client or connection logic
}
