package config_test

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "https://github.com/KiiChain/price-feeder/config"
)

// getProviderTimeout function here

func getProviderTimeout(providerName string, cfg *config.Config) time.Duration {
    for _, endpoint := range cfg.ProviderEndpoints {
        if endpoint.Name == providerName && endpoint.Timeout != "" {
            d, err := time.ParseDuration(endpoint.Timeout)
            if err == nil {
                return d
            }
        }
    }
    d, err := time.ParseDuration(cfg.ProviderTimeout)
    if err != nil {
        return 100 * time.Millisecond
    }
    return d
}

func TestProviderEndpointTimeout(t *testing.T) {
    cfg := config.Config{
        ProviderEndpoints: []config.ProviderEndpoint{
            {
                Name:     "binance",
                Rest:     "https://api1.binance.com",
                Websocket: "stream.binance.com:9443",
                Timeout:  "250ms",
            },
        },
        ProviderTimeout: "100ms",
    }
    timeout := getProviderTimeout("binance", &cfg)
    require.Equal(t, 250*time.Millisecond, timeout)
    timeout = getProviderTimeout("kraken", &cfg)
    require.Equal(t, 100*time.Millisecond, timeout)
}
