package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-playground/validator/v10"

	"cosmossdk.io/math"
)

const (
	DenomUSD = "USD"

	defaultProviderTimeout = 100 * time.Millisecond

	// API sources for oracle price feed - examples include price of BTC, ETH
	ProviderKraken   = "kraken"
	ProviderBinance  = "binance"
	ProviderCrypto   = "crypto"
	ProviderMexc     = "mexc"
	ProviderHuobi    = "huobi"
	ProviderOkx      = "okx"
	ProviderGate     = "gate"
	ProviderCoinbase = "coinbase"
	ProviderMock     = "mock"
)

var (
	// create a validator to user further and validate toml syntax
	validate = validator.New()

	// SupportedProviders is a mapping of all API sources for price feed
	SupportedProviders = map[string]struct{}{
		ProviderKraken:   {},
		ProviderBinance:  {},
		ProviderCrypto:   {},
		ProviderMexc:     {},
		ProviderOkx:      {},
		ProviderHuobi:    {},
		ProviderGate:     {},
		ProviderCoinbase: {},
		ProviderMock:     {},
	}

	// maxDeviationThreshold is the maxmimum allowed amount of standard
	// deviations which validators are able to set for a given asset.
	maxDeviationThreshold = math.LegacyMustNewDecFromStr("3.0")

	// SupportedQuotes defines a lookup table for which assets we support
	// using as quotes.
	SupportedQuotes = map[string]struct{}{
		DenomUSD:  {},
		"AXLUSDC": {},
		"USDC":    {},
		"USDT":    {},
		"DAI":     {},
		"BTC":     {},
		"ETH":     {},
		"ATOM":    {},
	}
)

type (
	// Config defines all necessary price-feeder configuration parameters.
	Config struct {
		Main              Main               `toml:"main" validate:"required,gt=0,dive,required"`
		Server            Server             `toml:"server" validate:"required,gt=0,dive,required"`
		CurrencyPairs     []CurrencyPair     `toml:"currency_pairs" validate:"required,gt=0,dive,required"`
		Deviations        []Deviation        `toml:"deviation_thresholds"`
		Account           Account            `toml:"account" validate:"required,gt=0,dive,required"`
		Keyring           Keyring            `toml:"keyring" validate:"required,gt=0,dive,required"`
		RPC               RPC                `toml:"rpc" validate:"required,gt=0,dive,required"`
		Telemetry         Telemetry          `toml:"telemetry"`
		Gas               Gas                `toml:"gas" validate:"required,gt=0,dive,required"`
		ProviderTimeout   string             `toml:"provider_timeout"`
		ProviderEndpoints []ProviderEndpoint `toml:"provider_endpoints" validate:"dive"`
		Healthchecks      []Healthchecks     `toml:"healthchecks" validate:"dive"`
	}

	// Main defines the main configuration parameters for the price-feeder
	Main struct {
		// EnableServer indicates whether the price-feeder server should be enabled
		EnableServer bool `toml:"enable_server" validate:"required"`
		// EnableVoting indicates whether the price-feeder should vote on prices
		EnableVoting bool `toml:"enable_voting" validate:"required"`
	}

	// Server defines the server configuration parameters for the price-feeder
	Server struct {
		// ListenAddress is the address on which the price-feeder server
		ListenAddress string `toml:"listen_addr" validate:"required"`
		// ReadTimeout is the maximum duration for reading the entire request
		ReadTimeout string `toml:"read_timeout" validate:"required"`
		// WriteTimeout is the maximum duration before timing out writes of the response
		WriteTimeout string `toml:"write_timeout" validate:"required"`
		// EnableCORS indicates whether CORS is enabled for the server.
		EnableCORS bool `toml:"enable_cors" validate:"required"`
		// AllowedOrigins defines the allowed origins for CORS requests.
		AllowedOrigins []string `toml:"allowed_origins"`
	}

	// Gas defines the gas adjustment and gas prices used for transactions.
	Gas struct {
		// GasAdjustment is a multiplier applied to the gas estimate to ensure
		// that the transaction has enough gas to be processed.
		GasAdjustment float64 `toml:"gas_adjustment" validate:"required"`

		// GasPrices defines the gas prices used for transactions.
		GasPrices string `toml:"gas_prices" validate:"required"`

		// GasLimit is the maximum amount of gas that can be used for a transaction.
		GasLimit uint64 `toml:"gas_limit" validate:"required"`
	}

	// CurrencyPair defines a price quote of the exchange rate for two different
	// currencies and the supported providers for getting the exchange rate.
	CurrencyPair struct {
		Base       string   `toml:"base" validate:"required"`
		ChainDenom string   `toml:"chain_denom" validate:"required"`
		Quote      string   `toml:"quote" validate:"required"`
		Providers  []string `toml:"providers" validate:"required,gt=0,dive,required"`
	}

	// Deviation defines a maximum amount of standard deviations that a given asset can
	// be from the median without being filtered out before voting.
	Deviation struct {
		Base      string `toml:"base" validate:"required"`
		Threshold string `toml:"threshold" validate:"required"`
	}

	// Account defines account related configuration that is related to the
	// network and transaction signing functionality.
	Account struct {
		ChainID    string `toml:"chain_id" validate:"required"`
		Address    string `toml:"address" validate:"required"`
		Validator  string `toml:"validator" validate:"required"`
		FeeGranter string `toml:"fee_granter"`
		Prefix     string `toml:"prefix" validate:"required"`
	}

	// Keyring defines the required keyring configuration.
	Keyring struct {
		Backend string `toml:"backend" validate:"required"`
		Dir     string `toml:"dir" validate:"required"`
	}

	// RPC defines RPC configuration of both the gRPC and Tendermint nodes.
	RPC struct {
		TMRPCEndpoint string `toml:"tmrpc_endpoint" validate:"required"`
		GRPCEndpoint  string `toml:"grpc_endpoint" validate:"required"`
		RPCTimeout    string `toml:"rpc_timeout" validate:"required"`
	}

	// Telemetry defines the configuration options for application telemetry.
	Telemetry struct {
		// Prefixed with keys to separate services
		ServiceName string `toml:"service_name" mapstructure:"service-name"`

		// Enabled enables the application telemetry functionality. When enabled,
		// an in-memory sink is also enabled by default. Operators may also enabled
		// other sinks such as Prometheus.
		Enabled bool `toml:"enabled" mapstructure:"enabled"`

		// Enable prefixing gauge values with hostname
		EnableHostname bool `toml:"enable_hostname" mapstructure:"enable-hostname"`

		// Enable adding hostname to labels
		EnableHostnameLabel bool `toml:"enable_hostname_label" mapstructure:"enable-hostname-label"`

		// Enable adding service to labels
		EnableServiceLabel bool `toml:"enable_service_label" mapstructure:"enable-service-label"`

		// GlobalLabels defines a global set of name/value label tuples applied to all
		// metrics emitted using the wrapper functions defined in telemetry package.
		//
		// Example:
		// [["chain_id", "cosmoshub-1"]]
		GlobalLabels [][]string `toml:"global_labels" mapstructure:"global-labels"`

		// PrometheusRetentionTime, when positive, enables a Prometheus metrics sink.
		// It defines the retention duration in seconds.
		PrometheusRetentionTime int64 `toml:"prometheus_retention" mapstructure:"prometheus-retention-time"`
	}

	// ProviderEndpoint defines an override setting in our config for the
	// hardcoded rest and websocket api endpoints.
	ProviderEndpoint struct {
		// Name of the provider, ex. "binance"
		Name string `toml:"name"`

		// Rest endpoint for the provider, ex. "https://api1.binance.com"
		Rest string `toml:"rest"`

		// Websocket endpoint for the provider, ex. "stream.binance.com:9443"
		Websocket string `toml:"websocket"`

		// Timeout for this provider, ex. "200ms" (optional, overrides global)
		Timeout string `toml:"timeout"`
	}

	Healthchecks struct {
		URL     string `toml:"url" validate:"required"`
		Timeout string `toml:"timeout" validate:"required"`
	}
)

// telemetryValidation is custom validation for the Telemetry struct.
func telemetryValidation(sl validator.StructLevel) {
	tel := sl.Current().Interface().(Telemetry)

	if tel.Enabled && (len(tel.GlobalLabels) == 0 || len(tel.ServiceName) == 0) {
		sl.ReportError(tel.Enabled, "enabled", "Enabled", "enabledNoOptions", "")
	}
}

// endpointValidation is custom validation for the ProviderEndpoint struct.
func endpointValidation(sl validator.StructLevel) {
	// validate the data type
	endpoint := sl.Current().Interface().(ProviderEndpoint)

	// must have at least one endpoint data
	if len(endpoint.Name) < 1 || len(endpoint.Rest) < 1 || len(endpoint.Websocket) < 1 {
		sl.ReportError(endpoint, "endpoint", "Endpoint", "unsupportedEndpointType", "")
	}

	// provider listed must be soported
	_, ok := SupportedProviders[endpoint.Name]
	if !ok {
		sl.ReportError(endpoint.Name, "name", "Name", "unsupportedEndpointProvider", "")
	}
}

// Validate returns an error if the Config object is invalid.
func (c Config) Validate() error {
	validate.RegisterStructValidation(telemetryValidation, Telemetry{})
	validate.RegisterStructValidation(endpointValidation, ProviderEndpoint{})
	return validate.Struct(c)
}

// ParseConfig attempts to read and parse configuration from the given file path.
// An error is returned if reading or parsing the config fails.
func ParseConfig(configPath string) (Config, error) {
	var cfg Config

	// validate the config path
	if configPath == "" {
		return cfg, errors.New("empty configuration file path")
	}

	// read config file from the given path
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config: %w", err)
	}

	// decode toml config
	_, err = toml.Decode(string(configData), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("failed to decode config: %w", err)
	}

	// set the default settings
	if len(cfg.ProviderTimeout) == 0 {
		cfg.ProviderTimeout = defaultProviderTimeout.String()
	}

	pairs := make(map[string]map[string]struct{})
	coinQuotes := make(map[string]struct{})

	// iterate over the currency pairs from the config
	for _, currencyPair := range cfg.CurrencyPairs {

		// save base on the pairs map
		_, ok := pairs[currencyPair.Base]
		if !ok {
			pairs[currencyPair.Base] = make(map[string]struct{})
		}

		// save the quote who are not USD (I must convert then on usd)
		if strings.ToUpper(currencyPair.Quote) != DenomUSD {
			coinQuotes[currencyPair.Quote] = struct{}{}
		}

		// validate if the selected quote is supported
		_, ok = SupportedQuotes[strings.ToUpper(currencyPair.Quote)]
		if !ok {
			return cfg, fmt.Errorf("unsupported quote: %s", currencyPair.Quote)
		}

		// iterate over the providers by currency
		for _, provider := range currencyPair.Providers {
			// validate the provider is supported
			_, ok = SupportedProviders[provider]
			if !ok {
				return cfg, fmt.Errorf("unsupported provider: %s", provider)
			}

			// save the providers by base denom
			pairs[currencyPair.Base][provider] = struct{}{}
		}
	}

	// Use coinQuotes to ensure that any quotes can be converted to USD.
	for quote := range coinQuotes {
		for index, pair := range cfg.CurrencyPairs {

			// validate I have the way to convert the quote to USD
			if pair.Base == quote && pair.Quote == DenomUSD {
				break
			}

			if index == len(cfg.CurrencyPairs)-1 {
				return cfg, fmt.Errorf("all non-usd quotes require a conversion rate feed")
			}
		}
	}

	// iterate over the pairs denom, check the minimum provider amount
	for base, providers := range pairs {
		// validate if we are mocking the provider
		_, ok := pairs[base]["mock"]
		if !ok && len(providers) < 3 {
			return cfg, fmt.Errorf("must have at least three providers for %s", base)
		}
	}

	// iterate over the deviation and check if valid
	for _, deviation := range cfg.Deviations {
		// validate the deviation threshold
		threshold, err := math.LegacyNewDecFromStr(deviation.Threshold)
		if err != nil {
			return cfg, fmt.Errorf("deviation thresholds must be numeric: %w", err)
		}

		// check deviation threshold value
		if threshold.GT(maxDeviationThreshold) {
			return cfg, fmt.Errorf("deviation thresholds must not exceed 3.0")
		}
	}

	return cfg, cfg.Validate()
}
