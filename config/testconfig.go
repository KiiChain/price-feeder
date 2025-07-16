package config

// ProviderEndpoint defines an override setting in our config for the
// hardcoded rest and websocket api endpoints.
type ProviderEndpoint struct {
	// Name of the provider, ex. "binance"
	Name string `toml:"name"`

	// Rest endpoint for the provider, ex. "https://api1.binance.com"
	Rest string `toml:"rest"`

	// Websocket endpoint for the provider, ex. "stream.binance.com:9443"
	Websocket string `toml:"websocket"`

	// Timeout for this provider, ex. "200ms" (optional, overrides global)
	Timeout string `toml:"timeout"`
}
