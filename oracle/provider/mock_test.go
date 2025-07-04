package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	"github.com/kiichain/price-feeder/oracle/types"
)

func TestMockProvider_GetTickerPrices(t *testing.T) {
	mp := NewMockProvider()

	t.Run("valid_request_single_ticker", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			require.Equal(t, "/", req.URL.String())
			resp := `Base,Quote,Price,Volume
UMEE,USDT,3.04,1827884.77
ATOM,USDC,21.84,1827884.77
`
			_, err := rw.Write([]byte(resp))
			require.NoError(t, err)
		}))
		defer server.Close()

		mp.client = server.Client()
		mp.baseURL = server.URL

		prices, err := mp.GetTickerPrices(types.CurrencyPair{Base: "UMEE", Quote: "USDT"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, math.LegacyMustNewDecFromStr("3.04"), prices["UMEEUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr("1827884.77"), prices["UMEEUSDT"].Volume)
	})

	t.Run("valid_request_multi_ticker", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			require.Equal(t, "/", req.URL.String())
			resp := `Base,Quote,Price,Volume
UMEE,USDT,3.04,1827884.77
ATOM,USDC,21.84,1827884.77
`
			_, err := rw.Write([]byte(resp))
			require.NoError(t, err)
		}))
		defer server.Close()

		mp.client = server.Client()
		mp.baseURL = server.URL

		prices, err := mp.GetTickerPrices(
			types.CurrencyPair{Base: "UMEE", Quote: "USDT"},
			types.CurrencyPair{Base: "ATOM", Quote: "USDC"},
		)
		require.NoError(t, err)
		require.Len(t, prices, 2)
		require.Equal(t, math.LegacyMustNewDecFromStr("3.04"), prices["UMEEUSDT"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr("1827884.77"), prices["UMEEUSDT"].Volume)
		require.Equal(t, math.LegacyMustNewDecFromStr("21.84"), prices["ATOMUSDC"].Price)
		require.Equal(t, math.LegacyMustNewDecFromStr("1827884.77"), prices["ATOMUSDC"].Volume)
	})

	t.Run("invalid_request_bad_response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			require.Equal(t, "/", req.URL.String())
			_, err := rw.Write([]byte(`FOO`))
			require.NoError(t, err)
		}))
		defer server.Close()

		mp.client = server.Client()
		mp.baseURL = server.URL

		prices, err := mp.GetTickerPrices(types.CurrencyPair{Base: "UMEE", Quote: "USDT"})
		require.Error(t, err)
		require.Nil(t, prices)
	})
}
