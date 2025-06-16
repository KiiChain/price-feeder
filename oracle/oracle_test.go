package oracle

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-metrics"
	oracletypes "github.com/kiichain/kiichain/v2/x/oracle/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/exp/slices"

	"cosmossdk.io/math"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/kiichain/price-feeder/config"
	"github.com/kiichain/price-feeder/oracle/client"
	"github.com/kiichain/price-feeder/oracle/provider"
	"github.com/kiichain/price-feeder/oracle/types"
)

type mockTelemetry struct {
	mx       sync.Mutex
	recorded []mockMetric
}

type mockMetric struct {
	keys   []string
	val    float32
	labels []metrics.Label
}

func resetMockTelemetry() *mockTelemetry {
	res := &mockTelemetry{
		mx: sync.Mutex{},
	}
	sendProviderFailureMetric = res.IncrCounterWithLabels
	return res
}

func (r mockMetric) containsLabel(expected metrics.Label) bool {
	for _, l := range r.labels {
		if l.Name == expected.Name && l.Value == expected.Value {
			return true
		}
	}
	return false
}

func (r mockMetric) labelsEqual(expected []metrics.Label) bool {
	if len(expected) != len(r.labels) {
		return false
	}
	for _, l := range expected {
		if !r.containsLabel(l) {
			return false
		}
	}
	return true
}

func (mt *mockTelemetry) IncrCounterWithLabels(keys []string, val float32, labels []metrics.Label) {
	mt.mx.Lock()
	defer mt.mx.Unlock()
	mt.recorded = append(mt.recorded, mockMetric{keys, val, labels})
}

func (mt *mockTelemetry) Len() int {
	return len(mt.recorded)
}

func (mt *mockTelemetry) AssertProviderError(t *testing.T, provider, base, reason, priceType string) {
	t.Helper()
	labels := []metrics.Label{
		{Name: "provider", Value: provider},
		{Name: "reason", Value: reason},
	}
	if base != "" {
		labels = append(labels, metrics.Label{Name: "base", Value: base})
	}
	if priceType != "" {
		labels = append(labels, metrics.Label{Name: "type", Value: priceType})
	}
	mt.AssertContains(t, []string{"failure", "provider"}, 1, labels)
}

func (mt *mockTelemetry) AssertContains(t *testing.T, keys []string, val float32, labels []metrics.Label) {
	t.Helper()
	for _, r := range mt.recorded {
		if r.val == val && slices.Equal(keys, r.keys) && r.labelsEqual(labels) {
			return
		}
	}
	require.Fail(t, fmt.Sprintf("no matching metric found: keys=%v, val=%v, labels=%v", keys, val, labels))
}

type mockProvider struct {
	prices    map[string]provider.TickerPrice
	candleErr error
}

func (m mockProvider) GetTickerPrices(_ ...types.CurrencyPair) (map[string]provider.TickerPrice, error) {
	return m.prices, nil
}

func (m mockProvider) GetCandlePrices(_ ...types.CurrencyPair) (map[string][]provider.CandlePrice, error) {
	if m.candleErr != nil {
		return nil, m.candleErr
	}
	candles := make(map[string][]provider.CandlePrice)
	for pair, price := range m.prices {
		candles[pair] = []provider.CandlePrice{
			{
				Price:     price.Price,
				TimeStamp: provider.PastUnixTime(1 * time.Minute),
				Volume:    price.Volume,
			},
		}
	}
	return candles, nil
}

func (m mockProvider) SubscribeCurrencyPairs(_ ...types.CurrencyPair) error {
	return nil
}

func (m mockProvider) GetAvailablePairs() (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}

type failingProvider struct {
	prices map[string]provider.TickerPrice
}

func (m failingProvider) GetTickerPrices(_ ...types.CurrencyPair) (map[string]provider.TickerPrice, error) {
	return nil, fmt.Errorf("unable to get ticker prices")
}

func (m failingProvider) GetCandlePrices(_ ...types.CurrencyPair) (map[string][]provider.CandlePrice, error) {
	return nil, fmt.Errorf("unable to get candle prices")
}

func (m failingProvider) SubscribeCurrencyPairs(_ ...types.CurrencyPair) error {
	return nil
}

func (m failingProvider) GetAvailablePairs() (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}

type OracleTestSuite struct {
	suite.Suite

	oracle *Oracle
}

// SetupSuite executes once before the suite's tests are executed.
func (ots *OracleTestSuite) SetupSuite() {
	ots.oracle = New(
		// set to debug to hit the debug-only code paths
		zerolog.Nop().Level(zerolog.DebugLevel),
		client.OracleClient{},
		[]config.CurrencyPair{
			{
				Base:       "UMEE",
				ChainDenom: "uumee",
				Quote:      "USDT",
				Providers:  []string{config.ProviderBinance},
			},
			{
				Base:       "UMEE",
				ChainDenom: "uumee",
				Quote:      "USDC",
				Providers:  []string{config.ProviderKraken},
			},
			{
				Base:       "XBT",
				ChainDenom: "uxbt",
				Quote:      "USDT",
				Providers:  []string{config.ProviderOkx},
			},
			{
				Base:       "USDC",
				ChainDenom: "uusdc",
				Quote:      "USD",
				Providers:  []string{config.ProviderHuobi},
			},
			{
				Base:       "USDT",
				ChainDenom: "uusdt",
				Quote:      "USD",
				Providers:  []string{config.ProviderCoinbase},
			},
		},
		time.Millisecond*100,
		make(map[string]math.LegacyDec),
		make(map[string]config.ProviderEndpoint),
		[]config.Healthchecks{
			{URL: "https://hc-ping.com/HEALTHCHECK-UUID", Timeout: "200ms"},
		},
	)
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, new(OracleTestSuite))
}

func (ots *OracleTestSuite) TestStop() {
	ots.Eventually(
		func() bool {
			ots.oracle.Stop()
			return true
		},
		5*time.Second,
		time.Second,
	)
}

func (ots *OracleTestSuite) TestPrices() {
	// initial prices should be empty (not set)
	ots.Require().Empty(ots.oracle.GetPrices())

	var denoms []string
	for _, v := range ots.oracle.chainDenomMapping {
		// we'll make ubxt a non-whitelisted denom
		if v != "uxbt" {
			denoms = append(denoms, v)
		}
	}
	ots.oracle.paramCache = ParamCache{
		params: &oracletypes.Params{
			Whitelist: denomList(denoms...),
		},
	}
	// Use a mock provider with exchange rates that are not specified in
	// configuration.
	ots.oracle.priceProviders = map[string]provider.Provider{
		config.ProviderBinance: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDX": {
					Price:  math.LegacyMustNewDecFromStr("3.72"),
					Volume: math.LegacyMustNewDecFromStr("2396974.02000000"),
				},
			},
		},
		config.ProviderKraken: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDX": {
					Price:  math.LegacyMustNewDecFromStr("3.70"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
	}
	telemetryMock := resetMockTelemetry()
	ots.Require().Error(ots.oracle.SetPrices(context.TODO()))
	ots.Require().Empty(ots.oracle.GetPrices())

	ots.Require().Equal(10, telemetryMock.Len())
	telemetryMock.AssertProviderError(ots.T(), config.ProviderBinance, "UMEE", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderKraken, "UMEE", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderOkx, "XBT", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderHuobi, "USDC", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderCoinbase, "USDT", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderBinance, "UMEE", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderKraken, "UMEE", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderOkx, "XBT", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderHuobi, "USDC", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderCoinbase, "USDT", "error", "candle")

	// use a mock provider without a conversion rate for these stablecoins
	ots.oracle.priceProviders = map[string]provider.Provider{
		config.ProviderBinance: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDT": {
					Price:  math.LegacyMustNewDecFromStr("3.72"),
					Volume: math.LegacyMustNewDecFromStr("2396974.02000000"),
				},
			},
		},
		config.ProviderKraken: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDC": {
					Price:  math.LegacyMustNewDecFromStr("3.70"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
	}
	telemetryMock = resetMockTelemetry()
	ots.Require().Error(ots.oracle.SetPrices(context.TODO()))
	ots.Require().Equal(6, telemetryMock.Len())
	telemetryMock.AssertProviderError(ots.T(), config.ProviderOkx, "XBT", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderHuobi, "USDC", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderCoinbase, "USDT", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderOkx, "XBT", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderHuobi, "USDC", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderCoinbase, "USDT", "error", "candle")

	prices := ots.oracle.GetPrices()
	ots.Require().Len(prices, 0)

	// use a mock provider to provide prices for the configured exchange pairs
	ots.oracle.priceProviders = map[string]provider.Provider{
		config.ProviderBinance: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDT": {
					Price:  math.LegacyMustNewDecFromStr("3.72"),
					Volume: math.LegacyMustNewDecFromStr("2396974.02000000"),
				},
			},
		},
		config.ProviderKraken: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDC": {
					Price:  math.LegacyMustNewDecFromStr("3.70"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
		config.ProviderHuobi: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDCUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("2396974.34000000"),
				},
			},
		},
		config.ProviderCoinbase: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDTUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
		config.ProviderOkx: mockProvider{
			prices: map[string]provider.TickerPrice{
				"XBTUSDT": {
					Price:  math.LegacyMustNewDecFromStr("3.717"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
	}

	telemetryMock = resetMockTelemetry()
	ots.Require().NoError(ots.oracle.SetPrices(context.TODO()))
	ots.Require().Equal(0, telemetryMock.Len())

	prices = ots.oracle.GetPrices()
	ots.Require().Len(prices, 4)
	ots.Require().Equal(math.LegacyMustNewDecFromStr("3.710916056220858266"), prices.AmountOf("uumee"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("3.717"), prices.AmountOf("uxbt"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdc"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdt"))

	// use one working provider and one provider with an incorrect exchange rate
	ots.oracle.priceProviders = map[string]provider.Provider{
		config.ProviderBinance: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDX": {
					Price:  math.LegacyMustNewDecFromStr("3.72"),
					Volume: math.LegacyMustNewDecFromStr("2396974.02000000"),
				},
			},
		},
		config.ProviderKraken: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDC": {
					Price:  math.LegacyMustNewDecFromStr("3.70"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
		config.ProviderHuobi: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDCUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("2396974.34000000"),
				},
			},
		},
		config.ProviderCoinbase: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDTUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
		config.ProviderOkx: mockProvider{
			prices: map[string]provider.TickerPrice{
				"XBTUSDT": {
					Price:  math.LegacyMustNewDecFromStr("3.717"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
	}

	telemetryMock = resetMockTelemetry()
	ots.Require().NoError(ots.oracle.SetPrices(context.TODO()))
	ots.Require().Equal(2, telemetryMock.Len())
	telemetryMock.AssertProviderError(ots.T(), config.ProviderBinance, "UMEE", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderBinance, "UMEE", "error", "candle")

	prices = ots.oracle.GetPrices()
	ots.Require().Len(prices, 4)
	ots.Require().Equal(math.LegacyMustNewDecFromStr("3.70"), prices.AmountOf("uumee"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("3.717"), prices.AmountOf("uxbt"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdc"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdt"))

	// use one working provider and one provider that fails
	ots.oracle.priceProviders = map[string]provider.Provider{
		config.ProviderBinance: failingProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDC": {
					Price:  math.LegacyMustNewDecFromStr("3.72"),
					Volume: math.LegacyMustNewDecFromStr("2396974.02000000"),
				},
			},
		},
		config.ProviderKraken: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDC": {
					Price:  math.LegacyMustNewDecFromStr("3.71"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
			candleErr: fmt.Errorf("test error"),
		},
		config.ProviderHuobi: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDCUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("2396974.34000000"),
				},
			},
		},
		config.ProviderCoinbase: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDTUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
		config.ProviderOkx: mockProvider{
			prices: map[string]provider.TickerPrice{
				"XBTUSDT": {
					Price:  math.LegacyMustNewDecFromStr("3.717"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
	}
	telemetryMock = resetMockTelemetry()
	ots.Require().NoError(ots.oracle.SetPrices(context.TODO()))
	ots.Require().Equal(3, telemetryMock.Len())
	telemetryMock.AssertProviderError(ots.T(), config.ProviderBinance, "UMEE", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderBinance, "UMEE", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderKraken, "UMEE", "error", "candle")

	prices = ots.oracle.GetPrices()
	ots.Require().Len(prices, 4)
	ots.Require().Equal(math.LegacyMustNewDecFromStr("3.71"), prices.AmountOf("uumee"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("3.717"), prices.AmountOf("uxbt"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdc"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdt"))

	// if a provider never initialized correctly, verify it doesn't prevent future updates
	ots.oracle.failedProviders = map[string]error{
		config.ProviderBinance: fmt.Errorf("test error"),
	}
	// a non-whitelisted entry fails (ubxt), but the rest succeed
	ots.oracle.priceProviders = map[string]provider.Provider{
		config.ProviderBinance: failingProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDC": {
					Price:  math.LegacyMustNewDecFromStr("3.72"),
					Volume: math.LegacyMustNewDecFromStr("2396974.02000000"),
				},
			},
		},
		config.ProviderKraken: mockProvider{
			prices: map[string]provider.TickerPrice{
				"UMEEUSDC": {
					Price:  math.LegacyMustNewDecFromStr("3.71"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
		config.ProviderHuobi: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDCUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("2396974.34000000"),
				},
			},
		},
		config.ProviderCoinbase: mockProvider{
			prices: map[string]provider.TickerPrice{
				"USDTUSD": {
					Price:  math.LegacyMustNewDecFromStr("1"),
					Volume: math.LegacyMustNewDecFromStr("1994674.34000000"),
				},
			},
		},
		config.ProviderOkx: failingProvider{},
	}
	telemetryMock = resetMockTelemetry()
	ots.Require().NoError(ots.oracle.SetPrices(context.TODO()))
	ots.Require().Equal(3, telemetryMock.Len())
	telemetryMock.AssertProviderError(ots.T(), config.ProviderOkx, "XBT", "error", "ticker")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderOkx, "XBT", "error", "candle")
	telemetryMock.AssertProviderError(ots.T(), config.ProviderBinance, "", "init", "")

	prices = ots.oracle.GetPrices()
	ots.Require().Len(prices, 3)
	ots.Require().Equal(math.LegacyMustNewDecFromStr("3.71"), prices.AmountOf("uumee"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdc"))
	ots.Require().Equal(math.LegacyMustNewDecFromStr("1"), prices.AmountOf("uusdt"))
}

func denomList(names ...string) oracletypes.DenomList {
	var result oracletypes.DenomList
	for _, n := range names {
		result = append(result, oracletypes.Denom{Name: n})
	}
	return result
}

func generateValidatorAddr() string {
	privKey := ed25519.GenPrivKey()
	pubKey := privKey.PubKey()
	return sdk.ValAddress(pubKey.Address().Bytes()).String()
}

func generateAcctAddr() string {
	privKey := ed25519.GenPrivKey()
	pubKey := privKey.PubKey()
	return sdk.AccAddress(pubKey.Address().Bytes()).String()
}

func TestTickScenarios(t *testing.T) {
	// generate address so that Bech32 address is valid
	validatorAddr := generateValidatorAddr()
	feederAddr := generateAcctAddr()

	tests := []struct {
		name               string
		isJailed           bool
		prices             map[string]math.LegacyDec
		pairs              []config.CurrencyPair
		whitelist          oracletypes.DenomList
		blockHeight        int64
		previousVotePeriod float64
		votePeriod         uint64
		mockBroadcastErr   error

		// expectations
		expectedVoteMsg *oracletypes.MsgAggregateExchangeRateVote
		expectedErr     error
	}{
		{
			name:               "Filtered prices, should broadcast all entries, none filtered",
			isJailed:           false,
			blockHeight:        1,
			previousVotePeriod: 0,
			votePeriod:         1,
			pairs: []config.CurrencyPair{
				{Base: "USDT", ChainDenom: "uusdt", Quote: "USD"},
				{Base: "BTC", ChainDenom: "ubtc", Quote: "USD"},
				{Base: "ETH", ChainDenom: "ueth", Quote: "USD"},
			},
			prices: map[string]math.LegacyDec{
				"USDT": math.LegacyMustNewDecFromStr("1.1"),
				"BTC":  math.LegacyMustNewDecFromStr("2.2"),
				"ETH":  math.LegacyMustNewDecFromStr("3.3"),
			},
			whitelist: denomList("uusdt", "ubtc", "ueth"),
			expectedVoteMsg: &oracletypes.MsgAggregateExchangeRateVote{
				ExchangeRates: "2.200000000000000000ubtc,3.300000000000000000ueth,1.100000000000000000uusdt",
				Feeder:        feederAddr,
				Validator:     validatorAddr,
			},
		},
		{
			name:               "Filtered prices, should broadcast only whitelisted entries",
			isJailed:           false,
			blockHeight:        1,
			previousVotePeriod: 0,
			votePeriod:         1,
			pairs: []config.CurrencyPair{
				{Base: "USDT", ChainDenom: "uusdt", Quote: "USD"},
				{Base: "BTC", ChainDenom: "ubtc", Quote: "USD"},
				{Base: "OTHER", ChainDenom: "uother", Quote: "USD"}, // filtered out
			},
			prices: map[string]math.LegacyDec{
				"USDT":  math.LegacyMustNewDecFromStr("1.1"),
				"BTC":   math.LegacyMustNewDecFromStr("2.2"),
				"OTHER": math.LegacyMustNewDecFromStr("3.3"),
			},
			whitelist: denomList("uusdt", "ubtc"),
			expectedVoteMsg: &oracletypes.MsgAggregateExchangeRateVote{
				ExchangeRates: "2.200000000000000000ubtc,1.100000000000000000uusdt", // does not include uother
				Feeder:        feederAddr,
				Validator:     validatorAddr,
			},
		},
		{
			name:               "Should not crash if broadcast returns nil response with error",
			isJailed:           false,
			blockHeight:        1,
			previousVotePeriod: 0,
			votePeriod:         1,
			pairs: []config.CurrencyPair{
				{Base: "USDT", ChainDenom: "uusdt", Quote: "USD"},
				{Base: "BTC", ChainDenom: "ubtc", Quote: "USD"},
				{Base: "ETH", ChainDenom: "ueth", Quote: "USD"},
			},
			prices: map[string]math.LegacyDec{
				"USDT": math.LegacyMustNewDecFromStr("1.1"),
				"BTC":  math.LegacyMustNewDecFromStr("2.2"),
				"ETH":  math.LegacyMustNewDecFromStr("3.3"),
			},
			whitelist: denomList("uusdt", "ubtc", "ueth"),
			expectedVoteMsg: &oracletypes.MsgAggregateExchangeRateVote{
				ExchangeRates: "2.200000000000000000ubtc,3.300000000000000000ueth,1.100000000000000000uusdt",
				Feeder:        feederAddr,
				Validator:     validatorAddr,
			},
			mockBroadcastErr: fmt.Errorf("test error"),
			expectedErr:      fmt.Errorf("test error"),
		},
		{
			name:               "Same voting period should avoid broadcasting without error",
			isJailed:           false,
			blockHeight:        1,
			previousVotePeriod: 1,
			votePeriod:         2,
			expectedErr:        nil,
			expectedVoteMsg:    nil,
		},
		{
			name:        "Jailed should return error",
			isJailed:    true,
			blockHeight: 1,
			expectedErr: fmt.Errorf("validator %s is jailed", validatorAddr),
		},
		{
			name:        "Zero block height should return error",
			isJailed:    false,
			blockHeight: 0,
			expectedErr: fmt.Errorf("expected positive block height"),
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		test := tc
		cdm, _ := createMappingsFromPairs(test.pairs)
		t.Run(test.name, func(t *testing.T) {
			var setPriceCount int
			var broadcastCount int
			// Create the oracle instance
			oracle := &Oracle{
				jailCache: JailCache{
					isJailed: test.isJailed,
				},
				mockSetPrices: func(ctx context.Context) error {
					setPriceCount++
					return nil
				},
				previousVotePeriod: test.previousVotePeriod,
				chainDenomMapping:  cdm,
				prices:             test.prices,
				paramCache: ParamCache{
					params: &oracletypes.Params{
						Whitelist:  test.whitelist,
						VotePeriod: test.votePeriod,
					},
				},
				oracleClient: client.OracleClient{
					OracleAddrString:    feederAddr,
					ValidatorAddrString: validatorAddr,
					MockBroadcastTx: func(ctx sdkclient.Context, msgs ...sdk.Msg) (*sdk.TxResponse, error) {
						// Assert that there's only one message
						require.Equal(t, 1, len(msgs))

						// Extract the message of type MsgAggregateExchangeRateVote
						voteMsg, ok := msgs[0].(*oracletypes.MsgAggregateExchangeRateVote)
						require.True(t, ok, "Expected message type *oracletypes.MsgAggregateExchangeRateVote")

						// Assert the expected values in the voteMsg
						require.Equal(t, test.expectedVoteMsg.ExchangeRates, voteMsg.ExchangeRates, test.name)
						require.Equal(t, test.expectedVoteMsg.Feeder, voteMsg.Feeder, test.name)
						require.Equal(t, test.expectedVoteMsg.Validator, voteMsg.Validator, test.name)

						broadcastCount++

						if test.mockBroadcastErr != nil {
							return nil, test.mockBroadcastErr
						}

						return &sdk.TxResponse{TxHash: "0xhash", Code: 200}, nil
					},
				},
			}

			// execute the tick function
			err := oracle.tick(ctx, sdkclient.Context{}, test.blockHeight)

			if test.expectedErr != nil {
				require.Equal(t, test.expectedErr, err, test.name)
			} else {
				require.NoError(t, err, test.name)
			}
			if test.expectedVoteMsg != nil {
				// ensure functions were actually called
				require.Equal(t, 1, broadcastCount, test.name)
				require.Equal(t, 1, setPriceCount, test.name)
			}
			if test.expectedVoteMsg == nil {
				// should not call broadcast
				require.Equal(t, 0, broadcastCount, test.name)
			}
		})
	}
}

func TestFilterPricesWithDenomList(t *testing.T) {
	tests := []struct {
		name           string
		inputDC        sdk.DecCoins
		inputDL        oracletypes.DenomList
		expectedResult sdk.DecCoins
	}{
		{
			name: "Matching denominations",
			inputDC: sdk.NewDecCoins(
				sdk.NewDecCoin("usdt", math.NewInt(100)),
				sdk.NewDecCoin("eth", math.NewInt(5)),
			),
			inputDL: oracletypes.DenomList{
				{Name: "usdt"},
			},
			expectedResult: sdk.NewDecCoins(
				sdk.NewDecCoin("usdt", math.NewInt(100)),
			),
		},
		{
			name: "No matching denominations",
			inputDC: sdk.NewDecCoins(
				sdk.NewDecCoin("btc", math.NewInt(1)),
				sdk.NewDecCoin("eth", math.NewInt(10)),
			),
			inputDL: oracletypes.DenomList{
				{Name: "usdt"},
			},
			expectedResult: sdk.NewDecCoins(),
		},
		{
			name:           "Empty input DecCoins and DenomList",
			inputDC:        sdk.DecCoins{},
			inputDL:        oracletypes.DenomList{},
			expectedResult: sdk.NewDecCoins(),
		},
		{
			name:    "Empty input DecCoins",
			inputDC: sdk.DecCoins{},
			inputDL: oracletypes.DenomList{
				{Name: "usdt"},
			},
			expectedResult: sdk.NewDecCoins(),
		},
		{
			name: "Empty input DenomList",
			inputDC: sdk.NewDecCoins(
				sdk.NewDecCoin("usdt", math.NewInt(100)),
				sdk.NewDecCoin("eth", math.NewInt(5)),
			),
			inputDL:        oracletypes.DenomList{},
			expectedResult: sdk.NewDecCoins(),
		},
	}

	for _, test := range tests {
		tc := test
		t.Run(tc.name, func(t *testing.T) {
			result := filterPricesByDenomList(tc.inputDC, tc.inputDL)
			require.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestGenerateExchangeRatesString(t *testing.T) {
	testCases := map[string]struct {
		input    sdk.DecCoins
		expected string
	}{
		"empty input": {
			input:    sdk.NewDecCoins(),
			expected: "",
		},
		"single denom": {
			input:    sdk.NewDecCoins(sdk.NewDecCoinFromDec("UMEE", math.LegacyMustNewDecFromStr("3.72"))),
			expected: "3.720000000000000000UMEE",
		},
		"multi denom": {
			input: sdk.NewDecCoins(sdk.NewDecCoinFromDec("UMEE", math.LegacyMustNewDecFromStr("3.72")),
				sdk.NewDecCoinFromDec("ATOM", math.LegacyMustNewDecFromStr("40.13")),
				sdk.NewDecCoinFromDec("OSMO", math.LegacyMustNewDecFromStr("8.69")),
			),
			expected: "40.130000000000000000ATOM,8.690000000000000000OSMO,3.720000000000000000UMEE",
		},
	}

	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			out := GenerateExchangeRatesString(tc.input)
			require.Equal(t, tc.expected, out)
		})
	}
}

func TestSuccessSetProviderTickerPricesAndCandles(t *testing.T) {
	providerPrices := make(provider.AggregatedProviderPrices, 1)
	providerCandles := make(provider.AggregatedProviderCandles, 1)
	pair := types.CurrencyPair{
		Base:  "ATOM",
		Quote: "USDT",
	}

	atomPrice := math.LegacyMustNewDecFromStr("29.93")
	atomVolume := math.LegacyMustNewDecFromStr("894123.00")

	prices := make(map[string]provider.TickerPrice, 1)
	prices[pair.String()] = provider.TickerPrice{
		Price:  atomPrice,
		Volume: atomVolume,
	}

	candles := make(map[string][]provider.CandlePrice, 1)
	candles[pair.String()] = []provider.CandlePrice{
		{
			Price:     atomPrice,
			Volume:    atomVolume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}

	success := SetProviderTickerPricesAndCandles(
		config.ProviderGate,
		providerPrices,
		providerCandles,
		prices,
		candles,
		pair,
	)

	require.True(t, success, "It should successfully set the prices")
	require.Equal(t, atomPrice, providerPrices[config.ProviderGate][pair.Base].Price)
	require.Equal(t, atomPrice, providerCandles[config.ProviderGate][pair.Base][0].Price)
}

func TestFailedSetProviderTickerPricesAndCandles(t *testing.T) {
	success := SetProviderTickerPricesAndCandles(
		config.ProviderCoinbase,
		make(provider.AggregatedProviderPrices, 1),
		make(provider.AggregatedProviderCandles, 1),
		make(map[string]provider.TickerPrice, 1),
		make(map[string][]provider.CandlePrice, 1),
		types.CurrencyPair{
			Base:  "ATOM",
			Quote: "USDT",
		},
	)

	require.False(t, success, "It should failed to set the prices, prices and candle are empty")
}

func TestSuccessGetComputedPricesCandles(t *testing.T) {
	providerCandles := make(provider.AggregatedProviderCandles, 1)
	pair := types.CurrencyPair{
		Base:  "ATOM",
		Quote: "USD",
	}

	atomPrice := math.LegacyMustNewDecFromStr("29.93")
	atomVolume := math.LegacyMustNewDecFromStr("894123.00")

	candles := make(map[string][]provider.CandlePrice, 1)
	candles[pair.Base] = []provider.CandlePrice{
		{
			Price:     atomPrice,
			Volume:    atomVolume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}
	providerCandles[config.ProviderBinance] = candles

	providerPair := map[string][]types.CurrencyPair{
		"binance": {pair},
	}

	prices, err := GetComputedPrices(
		zerolog.Nop(),
		providerCandles,
		make(provider.AggregatedProviderPrices, 1),
		providerPair,
		make(map[string]math.LegacyDec),
		map[string]struct{}{
			"ATOM": {},
		},
	)

	require.NoError(t, err, "It should successfully get computed candle prices")
	require.Equal(t, prices[pair.Base], atomPrice)
}

func TestSuccessGetComputedPricesTickers(t *testing.T) {
	providerPrices := make(provider.AggregatedProviderPrices, 1)
	pair := types.CurrencyPair{
		Base:  "ATOM",
		Quote: "USD",
	}

	atomPrice := math.LegacyMustNewDecFromStr("29.93")
	atomVolume := math.LegacyMustNewDecFromStr("894123.00")

	tickerPrices := make(map[string]provider.TickerPrice, 1)
	tickerPrices[pair.Base] = provider.TickerPrice{
		Price:  atomPrice,
		Volume: atomVolume,
	}
	providerPrices[config.ProviderBinance] = tickerPrices

	providerPair := map[string][]types.CurrencyPair{
		"binance": {pair},
	}

	prices, err := GetComputedPrices(
		zerolog.Nop(),
		make(provider.AggregatedProviderCandles, 1),
		providerPrices,
		providerPair,
		make(map[string]math.LegacyDec),
		map[string]struct{}{
			"ATOM": {},
		},
	)

	require.NoError(t, err, "It should successfully get computed ticker prices")
	require.Equal(t, prices[pair.Base], atomPrice)
}

func TestGetComputedPricesCandlesConversion(t *testing.T) {
	btcPair := types.CurrencyPair{
		Base:  "BTC",
		Quote: "ETH",
	}
	btcUSDPair := types.CurrencyPair{
		Base:  "BTC",
		Quote: "USD",
	}
	ethPair := types.CurrencyPair{
		Base:  "ETH",
		Quote: "USD",
	}
	btcEthPrice := math.LegacyMustNewDecFromStr("17.55")
	btcUSDPrice := math.LegacyMustNewDecFromStr("20962.601")
	ethUsdPrice := math.LegacyMustNewDecFromStr("1195.02")
	volume := math.LegacyMustNewDecFromStr("894123.00")
	providerCandles := make(provider.AggregatedProviderCandles, 4)

	// normal rates
	binanceCandles := make(map[string][]provider.CandlePrice, 2)
	binanceCandles[btcPair.Base] = []provider.CandlePrice{
		{
			Price:     btcEthPrice,
			Volume:    volume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}
	binanceCandles[ethPair.Base] = []provider.CandlePrice{
		{
			Price:     ethUsdPrice,
			Volume:    volume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}
	providerCandles[config.ProviderBinance] = binanceCandles

	// normal rates
	gateCandles := make(map[string][]provider.CandlePrice, 1)
	gateCandles[ethPair.Base] = []provider.CandlePrice{
		{
			Price:     ethUsdPrice,
			Volume:    volume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}
	gateCandles[btcPair.Base] = []provider.CandlePrice{
		{
			Price:     btcEthPrice,
			Volume:    volume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}
	providerCandles[config.ProviderGate] = gateCandles

	// abnormal eth rate
	okxCandles := make(map[string][]provider.CandlePrice, 1)
	okxCandles[ethPair.Base] = []provider.CandlePrice{
		{
			Price:     math.LegacyMustNewDecFromStr("1.0"),
			Volume:    volume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}
	providerCandles[config.ProviderOkx] = okxCandles

	// btc / usd rate
	krakenCandles := make(map[string][]provider.CandlePrice, 1)
	krakenCandles[btcUSDPair.Base] = []provider.CandlePrice{
		{
			Price:     btcUSDPrice,
			Volume:    volume,
			TimeStamp: provider.PastUnixTime(1 * time.Minute),
		},
	}
	providerCandles[config.ProviderKraken] = krakenCandles

	providerPair := map[string][]types.CurrencyPair{
		config.ProviderBinance: {btcPair, ethPair},
		config.ProviderGate:    {ethPair},
		config.ProviderOkx:     {ethPair},
		config.ProviderKraken:  {btcUSDPair},
	}

	prices, err := GetComputedPrices(
		zerolog.Nop(),
		providerCandles,
		make(provider.AggregatedProviderPrices, 1),
		providerPair,
		make(map[string]math.LegacyDec),
		map[string]struct{}{
			"BTC": {},
		},
	)

	require.NoError(t, err,
		"It should successfully filter out bad candles and convert everything to USD",
	)
	require.Equal(t,
		ethUsdPrice.Mul(
			btcEthPrice).Add(btcUSDPrice).Quo(math.LegacyMustNewDecFromStr("2")),
		prices[btcPair.Base],
	)
}

func TestGetComputedPricesTickersConversion(t *testing.T) {
	btcPair := types.CurrencyPair{
		Base:  "BTC",
		Quote: "ETH",
	}
	btcUSDPair := types.CurrencyPair{
		Base:  "BTC",
		Quote: "USD",
	}
	ethPair := types.CurrencyPair{
		Base:  "ETH",
		Quote: "USD",
	}
	volume := math.LegacyMustNewDecFromStr("881272.00")
	btcEthPrice := math.LegacyMustNewDecFromStr("72.55")
	ethUsdPrice := math.LegacyMustNewDecFromStr("9989.02")
	btcUSDPrice := math.LegacyMustNewDecFromStr("724603.401")
	providerPrices := make(provider.AggregatedProviderPrices, 1)

	// normal rates
	binanceTickerPrices := make(map[string]provider.TickerPrice, 2)
	binanceTickerPrices[btcPair.Base] = provider.TickerPrice{
		Price:  btcEthPrice,
		Volume: volume,
	}
	binanceTickerPrices[ethPair.Base] = provider.TickerPrice{
		Price:  ethUsdPrice,
		Volume: volume,
	}
	providerPrices[config.ProviderBinance] = binanceTickerPrices

	// normal rates
	gateTickerPrices := make(map[string]provider.TickerPrice, 4)
	gateTickerPrices[btcPair.Base] = provider.TickerPrice{
		Price:  btcEthPrice,
		Volume: volume,
	}
	gateTickerPrices[ethPair.Base] = provider.TickerPrice{
		Price:  ethUsdPrice,
		Volume: volume,
	}
	providerPrices[config.ProviderGate] = gateTickerPrices

	// abnormal eth rate
	okxTickerPrices := make(map[string]provider.TickerPrice, 1)
	okxTickerPrices[ethPair.Base] = provider.TickerPrice{
		Price:  math.LegacyMustNewDecFromStr("1.0"),
		Volume: volume,
	}
	providerPrices[config.ProviderOkx] = okxTickerPrices

	// btc / usd rate
	krakenTickerPrices := make(map[string]provider.TickerPrice, 1)
	krakenTickerPrices[btcUSDPair.Base] = provider.TickerPrice{
		Price:  btcUSDPrice,
		Volume: volume,
	}
	providerPrices[config.ProviderKraken] = krakenTickerPrices

	providerPair := map[string][]types.CurrencyPair{
		config.ProviderBinance: {ethPair, btcPair},
		config.ProviderGate:    {ethPair},
		config.ProviderOkx:     {ethPair},
		config.ProviderKraken:  {btcUSDPair},
	}

	prices, err := GetComputedPrices(
		zerolog.Nop(),
		make(provider.AggregatedProviderCandles, 1),
		providerPrices,
		providerPair,
		make(map[string]math.LegacyDec),
		map[string]struct{}{
			"BTC": {},
		},
	)

	require.NoError(t, err,
		"It should successfully filter out bad tickers and convert everything to USD",
	)
	require.Equal(t,
		ethUsdPrice.Mul(
			btcEthPrice).Add(btcUSDPrice).Quo(math.LegacyMustNewDecFromStr("2")),
		prices[btcPair.Base],
	)
}
