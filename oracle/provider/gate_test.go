package provider

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	"github.com/kiichain/price-feeder/config"
	"github.com/kiichain/price-feeder/oracle/types"
)

func TestGateProvider_GetTickerPrices(t *testing.T) {
	p, err := NewGateProvider(
		context.TODO(),
		zerolog.Nop(),
		config.ProviderEndpoint{},
		types.CurrencyPair{Base: "ATOM", Quote: "USDT"},
	)
	require.NoError(t, err)

	t.Run("valid_request_single_ticker", func(t *testing.T) {
		lastPrice := "34.69000000"
		volume := "2396974.02000000"

		tickerMap := map[string]GateTicker{}
		tickerMap["ATOM_USDT"] = GateTicker{
			Symbol: "ATOM_USDT",
			Last:   lastPrice,
			Vol:    volume,
		}

		p.tickers = tickerMap

		prices, err := p.GetTickerPrices(types.CurrencyPair{Base: "ATOM", Quote: "USDT"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, math.LegacyMustNewDecFromStr(lastPrice), prices["ATOMUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr(volume), prices["ATOMUSDT"].Volume)
	})

	t.Run("valid_request_multi_ticker", func(t *testing.T) {
		lastPriceAtom := "34.69000000"
		lastPriceUMEE := "41.35000000"
		volume := "2396974.02000000"

		tickerMap := map[string]GateTicker{}
		tickerMap["ATOM_USDT"] = GateTicker{
			Symbol: "ATOM_USDT",
			Last:   lastPriceAtom,
			Vol:    volume,
		}

		tickerMap["UMEE_USDT"] = GateTicker{
			Symbol: "UMEE_USDT",
			Last:   lastPriceUMEE,
			Vol:    volume,
		}

		p.tickers = tickerMap
		prices, err := p.GetTickerPrices(
			types.CurrencyPair{Base: "ATOM", Quote: "USDT"},
			types.CurrencyPair{Base: "UMEE", Quote: "USDT"},
		)
		require.NoError(t, err)
		require.Len(t, prices, 2)
		require.Equal(t, math.LegacyMustNewDecFromStr(lastPriceAtom), prices["ATOMUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr(volume), prices["ATOMUSDT"].Volume)
		require.Equal(t, math.LegacyMustNewDecFromStr(lastPriceUMEE), prices["UMEEUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr(volume), prices["UMEEUSDT"].Volume)
	})

	t.Run("invalid_request_invalid_ticker", func(t *testing.T) {
		prices, err := p.GetTickerPrices(types.CurrencyPair{Base: "FOO", Quote: "BAR"})
		require.NoError(t, err)
		require.Zero(t, len(prices))
	})
}

func TestGateProvider_SubscribeCurrencyPairs(t *testing.T) {
	p, err := NewGateProvider(
		context.TODO(),
		zerolog.Nop(),
		config.ProviderEndpoint{},
		types.CurrencyPair{Base: "ATOM", Quote: "USDT"},
	)
	require.NoError(t, err)

	t.Run("invalid_subscribe_channels_empty", func(t *testing.T) {
		err = p.SubscribeCurrencyPairs([]types.CurrencyPair{}...)
		require.ErrorContains(t, err, "currency pairs is empty")
	})
}

func TestGateCurrencyPairToGatePair(t *testing.T) {
	cp := types.CurrencyPair{Base: "ATOM", Quote: "USDT"}
	GateSymbol := currencyPairToGatePair(cp)
	require.Equal(t, GateSymbol, "ATOM_USDT")
}
