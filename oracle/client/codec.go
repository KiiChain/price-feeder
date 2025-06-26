package client

import (
	kiiparams "github.com/kiichain/kiichain/v3/app/params"

	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	evmcryptocodec "github.com/cosmos/evm/crypto/codec"
)

var encodingConfig kiiparams.EncodingConfig

func init() {
	encodingConfig = kiiparams.MakeEncodingConfig()

	// Register cosmos-sdk interfaces
	authtypes.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	// Register the pubkey for EVM and Cosmos interface
	evmcryptocodec.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	cryptocodec.RegisterInterfaces(encodingConfig.InterfaceRegistry)
}
