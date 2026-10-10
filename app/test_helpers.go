// Copyright 2022 Evmos Foundation
// This file is part of the Evmos Network packages.
//
// Evmos is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Evmos packages are distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Evmos packages. If not, see https://github.com/evmos/evmos/blob/main/LICENSE

package app

import (
	"encoding/json"
	"time"

	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/baseapp"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"

	"github.com/cometbft/cometbft/crypto/ed25519"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"cosmossdk.io/log"
	"cosmossdk.io/simapp"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/hoodium-io/hoodium/encoding"
	feemarkettypes "github.com/hoodium-io/hoodium/x/feemarket/types"

	"github.com/hoodium-io/hoodium/cmd/config"
	"github.com/hoodium-io/hoodium/utils"
)

func init() {
	cfg := sdk.GetConfig()
	config.SetBech32Prefixes(cfg)
	config.SetBip44CoinType(cfg)
}

// DefaultConsensusParams defines the default Tendermint consensus params used in
// Hoodium testing.
var DefaultConsensusParams = &tmproto.ConsensusParams{
	Block: &tmproto.BlockParams{
		MaxBytes: 200000,
		MaxGas:   10000000,
	},
	Evidence: &tmproto.EvidenceParams{
		MaxAgeNumBlocks: 302400,
		MaxAgeDuration:  504 * time.Hour, // 3 weeks is the max duration
		MaxBytes:        10000,
	},
	Validator: &tmproto.ValidatorParams{
		PubKeyTypes: []string{
			tmtypes.ABCIPubKeyTypeEd25519,
		},
	},
}

func init() {
	feemarkettypes.DefaultMinGasPrice = sdkmath.LegacyZeroDec()
	// Disable Hoodium's two-gas-price minimums by default in tests. These
	// minimums are a production fee policy; the general test suites exercise the
	// ante chain with small gas prices and are not concerned with the policy
	// itself. The policy is covered by dedicated tests in
	// app/ante/evm/deployment_fee_test.go.
	feemarkettypes.DefaultMinRegularGasPrice = sdkmath.LegacyZeroDec()
	feemarkettypes.DefaultMinDeploymentGasPrice = sdkmath.LegacyZeroDec()
	cfg := sdk.GetConfig()
	config.SetBech32Prefixes(cfg)
	config.SetBip44CoinType(cfg)
}

// Setup initializes a new Hoodium. A Nop logger is set in Hoodium.
func Setup(
	isCheckTx bool,
	feemarketGenesis *feemarkettypes.GenesisState,
) *Hoodium {
	privVal := ed25519.GenPrivKey()
	pubKey := privVal.PubKey()

	// create validator set with single validator
	validator := tmtypes.NewValidator(pubKey, 1)
	valSet := tmtypes.NewValidatorSet([]*tmtypes.Validator{validator})

	// generate genesis account
	senderPrivKey := secp256k1.GenPrivKey()
	acc := authtypes.NewBaseAccount(senderPrivKey.PubKey().Address().Bytes(), senderPrivKey.PubKey(), 0, 0)
	balance := banktypes.Balance{
		Address: acc.GetAddress().String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, sdkmath.NewInt(100000000000000))),
	}

	db := dbm.NewMemDB()

	chainID := utils.MainnetChainID + "-1"

	app := NewHoodium(
		log.NewNopLogger(),
		db,
		nil,
		true,
		map[int64]bool{},
		DefaultNodeHome,
		5,
		encoding.MakeConfig(ModuleBasics),
		simtestutil.NewAppOptionsWithFlagHome(DefaultNodeHome),
		baseapp.SetChainID(chainID),
	)
	if !isCheckTx {
		// init chain must be called to stop deliverState from being nil
		genesisState := NewDefaultGenesisState()

		genesisState = GenesisStateWithValSet(
			app,
			genesisState,
			acc.GetAddress(),
			valSet,
			[]authtypes.GenesisAccount{acc},
			balance,
		)

		// Verify feeMarket genesis
		if feemarketGenesis != nil {
			if err := feemarketGenesis.Validate(); err != nil {
				panic(err)
			}
			genesisState[feemarkettypes.ModuleName] = app.AppCodec().MustMarshalJSON(feemarketGenesis)
		}

		stateBytes, err := json.MarshalIndent(genesisState, "", " ")
		if err != nil {
			panic(err)
		}

		// Initialize the chain
		_, err = app.InitChain(
			&abci.RequestInitChain{
				ChainId:         chainID,
				Validators:      []abci.ValidatorUpdate{},
				ConsensusParams: DefaultConsensusParams,
				AppStateBytes:   stateBytes,
			},
		)
		if err != nil {
			panic(err)
		}
	}

	return app
}

func GenesisStateWithValSet(
	app *Hoodium,
	genesisState simapp.GenesisState,
	owner sdk.AccAddress,
	valSet *tmtypes.ValidatorSet,
	genAccs []authtypes.GenesisAccount,
	balances ...banktypes.Balance,
) simapp.GenesisState {
	// set genesis accounts
	authGenesis := authtypes.NewGenesisState(authtypes.DefaultParams(), genAccs)
	genesisState[authtypes.ModuleName] = app.AppCodec().MustMarshalJSON(authGenesis)

	validators := make([]stakingtypes.Validator, 0, len(valSet.Validators))
	delegations := make([]stakingtypes.Delegation, 0, len(valSet.Validators))

	for _, val := range valSet.Validators {
		pk, _ := cryptocodec.FromCmtPubKeyInterface(val.PubKey)
		operator := sdk.ValAddress(val.Address).String()

		validator, err := stakingtypes.NewValidator(
			operator,
			pk,
			stakingtypes.Description{},
		)
		if err != nil {
			panic(err)
		}

		// Bond the validator with a self-delegation of exactly one unit of
		// consensus power (Tokens == DefaultPowerReduction).
		validator.Status = stakingtypes.Bonded
		validator.Tokens = sdk.DefaultPowerReduction
		validator.DelegatorShares = sdkmath.LegacyNewDecFromInt(sdk.DefaultPowerReduction)

		validators = append(validators, validator)
		delegations = append(delegations, stakingtypes.Delegation{
			DelegatorAddress: sdk.AccAddress(val.Address).String(),
			ValidatorAddress: operator,
			Shares:           sdkmath.LegacyNewDecFromInt(sdk.DefaultPowerReduction),
		})
	}

	stakingGenesis := stakingtypes.DefaultGenesisState()
	stakingGenesis.Validators = validators
	stakingGenesis.Delegations = delegations
	stakingGenesis.LastTotalPower = sdkmath.NewInt(int64(len(validators)))
	genesisState[stakingtypes.ModuleName] = app.AppCodec().MustMarshalJSON(stakingGenesis)

	// Fund the bonded tokens pool so the staking InitGenesis invariant
	// (bonded pool balance == bonded coins) holds.
	bonded := sdk.NewCoin(
		sdk.DefaultBondDenom,
		sdk.DefaultPowerReduction.MulRaw(int64(len(validators))),
	)
	balances = append(balances, banktypes.Balance{
		Address: authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String(),
		Coins:   sdk.NewCoins(bonded),
	})

	totalSupply := sdk.NewCoins()
	for _, b := range balances {
		// add genesis acc tokens to total supply
		totalSupply = totalSupply.Add(b.Coins...)
	}

	// update total supply
	bankGenesis := banktypes.NewGenesisState(
		banktypes.DefaultGenesisState().Params,
		balances,
		totalSupply,
		[]banktypes.Metadata{},
		[]banktypes.SendEnabled{},
	)
	genesisState[banktypes.ModuleName] = app.AppCodec().MustMarshalJSON(bankGenesis)

	return genesisState
}
