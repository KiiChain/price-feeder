package provider

import (
	"context"
	"strconv"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	"github.com/kiichain/price-feeder/config"
	"github.com/kiichain/price-feeder/oracle/types"
)

func TestHuobiProvider_GetTickerPrices(t *testing.T) {
	p, err := NewHuobiProvider(
		context.TODO(),
		zerolog.Nop(),
		config.ProviderEndpoint{},
		types.CurrencyPair{Base: "ATOM", Quote: "USDT"},
	)
	require.NoError(t, err)

	t.Run("valid_request_single_ticker", func(t *testing.T) {
		lastPrice := 34.69000000
		volume := 2396974.02000000

		tickerMap := map[string]HuobiTicker{}
		tickerMap["market.atomusdt.ticker"] = HuobiTicker{
			CH: "market.atomusdt.ticker",
			Tick: HuobiTick{
				LastPrice: lastPrice,
				Vol:       volume,
			},
		}

		p.tickers = tickerMap

		prices, err := p.GetTickerPrices(types.CurrencyPair{Base: "ATOM", Quote: "USDT"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, math.LegacyMustNewDecFromStr(strconv.FormatFloat(lastPrice, 'f', -1, 64)), prices["ATOMUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr(strconv.FormatFloat(volume, 'f', -1, 64)), prices["ATOMUSDT"].Volume)
	})

	t.Run("valid_request_multi_ticker", func(t *testing.T) {
		lastPriceAtom := 34.69000000
		lastPriceKii := 41.35000000
		volume := 2396974.02000000

		tickerMap := map[string]HuobiTicker{}
		tickerMap["market.atomusdt.ticker"] = HuobiTicker{
			CH: "market.atomusdt.ticker",
			Tick: HuobiTick{
				LastPrice: lastPriceAtom,
				Vol:       volume,
			},
		}

		tickerMap["market.kiiusdt.ticker"] = HuobiTicker{
			CH: "market.kiiusdt.ticker",
			Tick: HuobiTick{
				LastPrice: lastPriceKii,
				Vol:       volume,
			},
		}

		p.tickers = tickerMap
		prices, err := p.GetTickerPrices(
			types.CurrencyPair{Base: "ATOM", Quote: "USDT"},
			types.CurrencyPair{Base: "KII", Quote: "USDT"},
		)
		require.NoError(t, err)
		require.Len(t, prices, 2)
		require.Equal(t, math.LegacyMustNewDecFromStr(strconv.FormatFloat(lastPriceAtom, 'f', -1, 64)), prices["ATOMUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr(strconv.FormatFloat(volume, 'f', -1, 64)), prices["ATOMUSDT"].Volume)
		require.Equal(t, math.LegacyMustNewDecFromStr(strconv.FormatFloat(lastPriceKii, 'f', -1, 64)), prices["KIIUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr(strconv.FormatFloat(volume, 'f', -1, 64)), prices["KIIUSDT"].Volume)
	})

	t.Run("invalid_request_invalid_ticker", func(t *testing.T) {
		prices, err := p.GetTickerPrices(types.CurrencyPair{Base: "FOO", Quote: "BAR"})
		require.NoError(t, err)
		require.Zero(t, len(prices))
	})
}

func TestHuobiProvider_SubscribeCurrencyPairs(t *testing.T) {
	p, err := NewHuobiProvider(
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

func TestHuobiCurrencyPairToHuobiPair(t *testing.T) {
	cp := types.CurrencyPair{Base: "ATOM", Quote: "USDT"}
	binanceSymbol := currencyPairToHuobiTickerPair(cp)
	require.Equal(t, binanceSymbol, "market.atomusdt.ticker")
}
