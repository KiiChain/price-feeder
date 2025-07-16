package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProviderEndpointTimeout(t *testing.T) {
	cfg := Config{
		ProviderEndpoints: []ProviderEndpoint{
			{
				Name:     "binance",
				Rest:     "https://api1.binance.com",
				Websocket: "stream.binance.com:9443",
				Timeout:  "250ms",
			},
		},
		ProviderTimeout: "100ms",
	}
	timeout := GetProviderTimeout("binance", &cfg)
	require.Equal(t, 250*time.Millisecond, timeout)
	timeout = GetProviderTimeout("kraken", &cfg)
	require.Equal(t, 100*time.Millisecond, timeout)
}
