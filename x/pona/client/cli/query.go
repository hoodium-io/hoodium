// Package cli provides the `runed query pona` command tree.
//
// # Why these commands read the module store directly
//
// x/pona exposes no gRPC Query service: its only protobuf file is genesis.proto,
// so there is nothing for AutoCLI or a generated QueryClient to call. The
// module's state is still fully queryable, because every Cosmos SDK store is
// served through the `store/<module>/key` ABCI path. Reading the two keys the
// module keeps (params and the current block's transaction count), plus the
// validator reward pool's bank balance, therefore needs no new protobuf, no new
// gRPC service and no consensus change.
//
// The params value is the binary protobuf encoding of types.Params (confirmed
// against a live node: key 0x01 returns a protobuf payload). It is decoded with
// the application's binary codec, not with JSON.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/hoodium-io/hoodium/x/pona/keeper"
	"github.com/hoodium-io/hoodium/x/pona/types"
)

const (
	// targetBlockTime is Hoodium's target block interval. The emission
	// schedule's tier boundaries are defined in terms of it (see
	// types.BlocksPerYearAt6s), so duration estimates use the same constant.
	targetBlockTime = 6 * time.Second

	// flagTxCount is the flag accepted by `query pona block-reward`.
	flagTxCount = "tx-count"
)

// printOutput renders v honouring the command's --output flag: JSON for "json",
// and the JSON converted to YAML for "text" (the SDK default), matching what
// client.Context.PrintProto does for protobuf messages.
func printOutput(clientCtx client.Context, v interface{}) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	return clientCtx.PrintRaw(raw)
}

// runeString formats an arune amount as a decimal RUNE string for readability.
//
// It delegates to types.RuneString so the CLI and the node's reward log render
// amounts identically; there is exactly one implementation of the conversion.
func runeString(amount sdkmath.Int) string {
	return types.RuneString(amount)
}

// durationFromBlocks converts a block count into an approximate wall-clock
// duration at the 6s target block time.
func durationFromBlocks(blocks uint64) string {
	return humanDuration(time.Duration(blocks) * targetBlockTime) //nolint:gosec // bounded by the schedule
}

// humanDuration renders a duration with at most two units, which keeps the
// output readable for the multi-year horizons an emission schedule has.
func humanDuration(d time.Duration) string {
	const (
		day  = 24 * time.Hour
		year = 365 * day
	)

	switch {
	case d <= 0:
		return "0s"
	case d >= year:
		return fmt.Sprintf("%dy %dd", int64(d/year), int64((d%year)/day))
	case d >= day:
		return fmt.Sprintf("%dd %dh", int64(d/day), int64((d%day)/time.Hour))
	default:
		return d.String()
	}
}

// ---------------------------------------------------------------------------
// store access
// ---------------------------------------------------------------------------

// queryStoreKey reads a raw key from the pona module's KV store at the height
// configured on clientCtx (0 = latest).
//
// client.Context.QueryStore is the SDK's purpose-built helper for this: it issues
// the `store/<module>/key` ABCI query. A missing key is not an error: the store
// answers with an empty value.
func queryStoreKey(clientCtx client.Context, key []byte) ([]byte, int64, error) {
	return clientCtx.QueryStore(key, types.StoreKey)
}

// queryStoreKeyAt reads a raw key from the pona store at a specific height.
// A height of 0 means "latest" (the node's default).
func queryStoreKeyAt(clientCtx client.Context, key []byte, height int64) ([]byte, int64, error) {
	return clientCtx.WithHeight(height).QueryStore(key, types.StoreKey)
}

// queryParams returns the module params together with the height they were read
// at. If the store holds no params yet, the compile-time defaults are returned
// so the command still answers.
func queryParams(clientCtx client.Context) (types.Params, int64, error) {
	value, height, err := queryStoreKey(clientCtx, types.KeyPrefixParams)
	if err != nil {
		return types.Params{}, 0, err
	}

	if len(value) == 0 {
		// No params in the store yet (e.g. a node that has not finished
		// InitGenesis). Fall back to the module's own default genesis so the
		// command answers with the schedule the module itself would install,
		// rather than a denom literal duplicated here.
		return types.DefaultGenesisState().Params, height, nil
	}

	var params types.Params
	if err := clientCtx.Codec.Unmarshal(value, &params); err != nil {
		return types.Params{}, 0, fmt.Errorf("failed to decode pona params: %w", err)
	}

	return params, height, nil
}

// queryBlockTxCount returns the transaction count recorded for the last
// finalized block. It is 0 when no count has been recorded yet.
func queryBlockTxCount(clientCtx client.Context) (uint64, error) {
	return queryBlockTxCountAt(clientCtx, clientCtx.Height)
}

// queryBlockTxCountAt returns the PoNA transaction count recorded at the given
// height. A height of 0 means "latest".
func queryBlockTxCountAt(clientCtx client.Context, height int64) (uint64, error) {
	value, _, err := queryStoreKeyAt(clientCtx, types.KeyBlockTxCount, height)
	if err != nil {
		return 0, err
	}

	if len(value) == 0 {
		return 0, nil
	}

	return sdk.BigEndianToUint64(value), nil
}

// queryBlockTxEntries returns the number of entries physically present in the
// block at the given height (0 = latest), i.e. exactly what CometBFT reports as
// num_txs.
//
// It is the raw count, so it INCLUDES the app-level injected pseudo-transaction,
// unlike queryBlockTxCountAt. Reporting both is what lets an operator reconcile
// the node log ("num_txs=1") with PoNA ("tx_count=0") without guessing.
func queryBlockTxEntries(ctx context.Context, clientCtx client.Context, height int64) (uint64, error) {
	node, err := clientCtx.GetNode()
	if err != nil {
		return 0, err
	}

	var queryHeight *int64
	if height > 0 {
		queryHeight = &height
	}

	res, err := node.Block(ctx, queryHeight)
	if err != nil {
		return 0, err
	}

	if res == nil || res.Block == nil {
		return 0, fmt.Errorf("node returned an empty block at height %d", height)
	}

	return uint64(len(res.Block.Txs)), nil //nolint:gosec // bounded by the block
}

// queryPoolBalance returns the validator reward pool balance in the given denom.
//
// The bank query goes through clientCtx, which uses the configured gRPC endpoint
// when there is one and falls back to an ABCI query otherwise - so the command
// works on a stock validator with no gRPC client configured.
func queryPoolBalance(ctx context.Context, clientCtx client.Context, denom string) (sdk.Coin, error) {
	address := authtypes.NewModuleAddress(types.ValidatorRewardPoolName)

	resp, err := banktypes.NewQueryClient(clientCtx).Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: address.String(),
		Denom:   denom,
	})
	if err != nil {
		return sdk.Coin{}, err
	}

	if resp.Balance == nil {
		return sdk.NewCoin(denom, sdkmath.ZeroInt()), nil
	}

	return *resp.Balance, nil
}

// ---------------------------------------------------------------------------
// commands
// ---------------------------------------------------------------------------

// GetQueryCmd returns the parent command for all x/pona CLI query commands.
func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Querying commands for the pona (RUNE block rewards) module",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		GetParamsCmd(),
		GetPoolCmd(),
		GetTierCmd(),
		GetBlockRewardCmd(),
		GetTxCountCmd(),
	)

	return cmd
}

// GetParamsCmd returns the full pona parameter set: the emission schedule and
// the PoNA activity rates for every tier.
func GetParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "params",
		Short: "Show the RUNE emission schedule and PoNA activity rates",
		Long: `Show the pona module parameters.

The output lists the ordered emission schedule. For every tier it reports the
block-height window the tier covers and the graded Proof of Network Activity rate
ladder that applies inside it:

  max     paid when tx_count >= max_tx_count_threshold
  mid     paid when mid_tx_count_threshold <= tx_count < max
  low     paid when low_tx_count_threshold <= tx_count < mid
  single  paid when the block contains exactly 1 transaction
  zero    paid when the block contains no regular transactions

A max_tx_count_threshold of 0 means PoNA is disabled for that tier and the max
rate is always paid. Amounts are reported in arune (1 RUNE = 10^18 arune) together
with the human-readable RUNE value.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			params, height, err := queryParams(clientCtx)
			if err != nil {
				return err
			}

			return printOutput(clientCtx, newParamsReport(params, height))
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetPoolCmd returns the validator reward pool balance: the RUNE reserve that
// still has to be emitted.
func GetPoolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "Show the validator reward pool balance (RUNE left to emit)",
		Long: `Show the balance of the validator_reward_pool module account.

Every block's reward is paid out of this pre-funded reserve. When it reaches
zero, emission stops and the chain runs on transaction fees only ("fee-only"),
which is the intended end state.

remaining_blocks is computed at the active tier's FULL rate, so it is a lower
bound on the reserve's lifetime: quiet blocks pay the low-activity or the
zero-activity rate and make the reserve last longer.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			params, height, err := queryParams(clientCtx)
			if err != nil {
				return err
			}

			balance, err := queryPoolBalance(cmd.Context(), clientCtx, params.Denom)
			if err != nil {
				return err
			}

			report := poolReport{
				Height:        height,
				ModuleAccount: types.ValidatorRewardPoolName,
				Address:       authtypes.NewModuleAddress(types.ValidatorRewardPoolName).String(),
				Denom:         params.Denom,
				Balance:       balance.Amount.String(),
				BalanceRune:   runeString(balance.Amount),
				TierIndex:     -1,
				// Defaults for the case where no tier covers the height: there is
				// no rate left to divide by (emission has ended, fee-only).
				RemainingBlocks:   "none",
				RemainingDuration: "none",
			}

			idx, tier, ok := params.TierForHeight(uint64(height)) //nolint:gosec // height >= 0
			if ok && tier.MaxActivityReward.IsPositive() {
				remaining := balance.Amount.Quo(tier.MaxActivityReward)
				blocks := remaining.BigInt().Uint64() //nolint:gosec // reserve / rate fits in uint64

				report.TierIndex = idx
				report.MaxRewardPerBlock = tier.MaxActivityReward.String()
				report.MaxRewardRune = runeString(tier.MaxActivityReward)
				report.RemainingBlocks = remaining.String()
				report.RemainingDuration = durationFromBlocks(blocks)
			} else {
				report.EmissionEnded = true
			}

			return printOutput(clientCtx, report)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetTierCmd returns the tier that is active at a height, plus how far away the
// next tier (with its new rates) is.
func GetTierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tier",
		Short: "Show the active emission tier and the blocks left before the next one",
		Long: `Show the emission-schedule tier that is active at the queried height.

The height defaults to the node's latest height; pass --height to ask about a
specific block height instead.

The output reports the blocks remaining before the next tier starts, the
approximate time that represents at the 6s target block time, and the rates that
will apply once it does. The final tier is open-ended (end_height 0), so it has
no successor and "unbounded" is reported.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			params, latestHeight, err := queryParams(clientCtx)
			if err != nil {
				return err
			}

			height := resolveHeight(clientCtx, latestHeight)
			if height < 0 {
				return fmt.Errorf("invalid height %d", height)
			}

			idx, tier, ok := params.TierForHeight(uint64(height))
			if !ok {
				return fmt.Errorf("no emission tier covers height %d: emission has ended (fee-only)", height)
			}

			report := tierReport{
				Height: height,
				View: scheduleView{
					TierIndex: idx,
					TierCount: len(params.Tiers),
				},
			}
			report.View.fillFromTier(tier)

			if tier.EndHeight == 0 {
				report.BlocksUntilNextTier = "unbounded"
				report.TimeUntilNextTier = "unbounded"
			} else {
				blocks := tier.EndHeight - uint64(height) //nolint:gosec // height < EndHeight here

				report.NextTierStartHeight = tier.EndHeight
				report.BlocksUntilNextTier = strconv.FormatUint(blocks, 10)
				report.TimeUntilNextTier = durationFromBlocks(blocks)

				if idx+1 < len(params.Tiers) {
					next := params.Tiers[idx+1]
					report.NextTierMaxReward = next.MaxActivityReward.String()
					report.NextTierMaxRewardRune = runeString(next.MaxActivityReward)
					report.NextTierMidReward = next.MidActivityReward.String()
					report.NextTierMidRewardRune = runeString(next.MidActivityReward)
					report.NextTierLowReward = next.LowActivityReward.String()
					report.NextTierLowRewardRune = runeString(next.LowActivityReward)
					report.NextTierZeroReward = next.ZeroActivityReward.String()
					report.NextTierZeroRewardRune = runeString(next.ZeroActivityReward)
				}
			}

			return printOutput(clientCtx, report)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetBlockRewardCmd answers "what does a block pay right now, and why?".
func GetBlockRewardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block-reward",
		Short: "Show the RUNE reward a block pays, and why",
		Long: `Show the RUNE block reward that applies at the queried height.

The reward is a function of two things: the emission tier covering the height,
and the block's Proof of Network Activity band. The band is selected by the
number of REGULAR transactions the block contains; the app-level injected
pseudo-transaction that vote extensions add to every block is NOT counted.

By default the command reports the block that was just paid, using the
transaction count the chain recorded for the last finalized block. Pass
--tx-count N to ask "what would a block with N transactions pay?" without any
such block existing, and --height to ask about another height.

The output shows the whole rate ladder of the active tier (max, mid, low, single,
zero) with its thresholds, alongside the band that is currently in effect, so it
is clear what the neighbouring bands would pay.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			requested, err := cmd.Flags().GetInt64(flagTxCount)
			if err != nil {
				return err
			}
			if requested < -1 {
				return fmt.Errorf("--%s must be -1 (use the chain's last block) or >= 0, got %d", flagTxCount, requested)
			}

			params, latestHeight, err := queryParams(clientCtx)
			if err != nil {
				return err
			}

			height := resolveHeight(clientCtx, latestHeight)
			if height < 0 {
				return fmt.Errorf("invalid height %d", height)
			}

			txCount := uint64(requested) //nolint:gosec // validated >= 0 in this branch
			txCountSource := "flag"
			if requested < 0 {
				txCount, err = queryBlockTxCount(clientCtx)
				if err != nil {
					return err
				}
				txCountSource = "chain (recorded for the last finalized block)"
			}

			breakdown := keeper.ComputeBlockReward(params, uint64(height), txCount)

			report := blockRewardReport{
				Height:        height,
				TxCount:       txCount,
				TxCountSource: txCountSource,
				Band:          string(breakdown.Band),
				Reward:        breakdown.Reward.String(),
				RewardRune:    runeString(breakdown.Reward),
				Denom:         params.Denom,
				View: scheduleView{
					TierIndex: breakdown.TierIndex,
					TierCount: len(params.Tiers),
				},
			}

			if breakdown.HasTier() {
				report.View.fillFromTier(breakdown.Tier)
				report.BandExplanation = explainBand(breakdown.Tier, txCount)
			} else {
				report.BandExplanation = "no emission tier covers this height: emission has ended (fee-only)."
			}

			// Pool context: whether the reserve can actually pay this block.
			balance, balanceErr := queryPoolBalance(cmd.Context(), clientCtx, params.Denom)
			if balanceErr != nil {
				report.PoolError = balanceErr.Error()
			} else {
				report.PoolBalance = balance.Amount.String()
				report.PoolBalanceRune = runeString(balance.Amount)
				report.PoolSufficient = balance.Amount.GTE(breakdown.Reward)
			}

			return printOutput(clientCtx, report)
		},
	}

	cmd.Flags().Int64(
		flagTxCount,
		-1,
		"Number of regular transactions to assume for the block (-1 = use the count the chain recorded for the last finalized block)",
	)
	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetTxCountCmd returns the transaction count the chain recorded for the last
// finalized block, next to the block's raw entry count.
func GetTxCountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx-count",
		Short: "Show the PoNA transaction count vs the block's raw entry count",
		Long: `Show the transaction count x/pona used to select the last block's reward.

Two counts are reported, because they answer different questions:

  tx_count        what PoNA counted: REGULAR transactions only. This is the number
                  the node stored in PreBlock and the one that selected the band.
  block_tx_count  every ENTRY in the block, exactly as CometBFT reports it. It
                  includes the app-level injected pseudo-transaction that carries
                  vote extensions, because that entry is physically in the block.

They normally differ by exactly 1 on a vote-extension chain (block_tx_count =
tx_count + 1), and block_tx_count is what CometBFT logs as num_txs. Seeing
block_tx_count=1 with tx_count=0 is therefore the expected, healthy idle case.

Both counts are reported for the height that was queried (default: latest), so
use --height to inspect an older block.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			params, latestHeight, err := queryParams(clientCtx)
			if err != nil {
				return err
			}

			height := resolveHeight(clientCtx, latestHeight)

			// The PoNA count is stored for the block being finalised, so read it
			// from the state at that height.
			txCount, err := queryBlockTxCountAt(clientCtx, height)
			if err != nil {
				return err
			}

			report := txCountReport{
				Height:      height,
				TxCount:     txCount,
				TierIndex:   -1,
				Band:        string(types.BandNone),
				Reward:      "0",
				RewardRune:  "0",
				TxCountNote: "regular transactions only; the injected vote-extension pseudo-tx is excluded",
			}

			// The raw block gives the honest "how many entries were in this block"
			// number, the one CometBFT logs as num_txs.
			if rawCount, err := queryBlockTxEntries(cmd.Context(), clientCtx, height); err == nil {
				report.BlockTxCount = rawCount
				report.BlockTxCountNote = "every entry in the block, including the injected pseudo-tx (CometBFT num_txs)"
			} else {
				report.BlockTxCountError = err.Error()
			}

			if height >= 0 {
				if idx, tier, ok := params.TierForHeight(uint64(height)); ok {
					breakdown := keeper.ComputeBlockReward(params, uint64(height), txCount)

					report.TierIndex = idx
					report.Band = string(tier.BandForTxCount(txCount))
					report.LowTxCountThreshold = tier.LowTxCountThreshold
					report.MidTxCountThreshold = tier.MidTxCountThreshold
					report.MaxTxCountThreshold = tier.MaxTxCountThreshold
					report.Reward = breakdown.Reward.String()
					report.RewardRune = runeString(breakdown.Reward)
				}
			}

			return printOutput(clientCtx, report)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// ---------------------------------------------------------------------------
// output types
// ---------------------------------------------------------------------------

// resolveHeight picks the height to report on: the --height flag when set,
// otherwise the node's latest height.
func resolveHeight(clientCtx client.Context, latest int64) int64 {
	if clientCtx.Height > 0 {
		return clientCtx.Height
	}

	return latest
}

// amountReport carries an amount in both arune (exact) and RUNE (readable).
type amountReport struct {
	Arune string `json:"arune" yaml:"arune"`
	Rune  string `json:"rune" yaml:"rune"`
}

// newAmountReport renders an amount as an amountReport.
func newAmountReport(amount sdkmath.Int) amountReport {
	return amountReport{
		Arune: amount.String(),
		Rune:  runeString(amount),
	}
}

// tierRates is the PoNA rate set of a single tier, in the order the bands are
// applied.
type tierRates struct {
	// Thresholds, ascending: low < mid < max.
	LowTxCountThreshold uint64 `json:"low_tx_count_threshold" yaml:"low_tx_count_threshold"`
	MidTxCountThreshold uint64 `json:"mid_tx_count_threshold" yaml:"mid_tx_count_threshold"`
	MaxTxCountThreshold uint64 `json:"max_tx_count_threshold" yaml:"max_tx_count_threshold"`
	// PoNAEnabled is false when MaxTxCountThreshold is 0: the max rate always applies.
	PoNAEnabled bool `json:"pona_enabled" yaml:"pona_enabled"`
	// Max is paid when tx_count >= max_tx_count_threshold.
	Max amountReport `json:"max" yaml:"max"`
	// Mid is paid when mid <= tx_count < max.
	Mid amountReport `json:"mid" yaml:"mid"`
	// Low is paid when low <= tx_count < mid.
	Low amountReport `json:"low" yaml:"low"`
	// Single is the rate for a block with exactly one transaction.
	Single amountReport `json:"single" yaml:"single"`
	// Zero is the configured zero-activity rate. It is also the FLOOR: no block
	// is ever paid less than this.
	Zero amountReport `json:"zero" yaml:"zero"`
	// FloorApplied is true when the floor (rather than a band's own rate) is
	// what a block in that band is paid.
	FloorApplied bool `json:"floor_applied" yaml:"floor_applied"`
}

// newTierRates extracts the rate ladder of a tier.
func newTierRates(tier types.Tier) tierRates {
	// floorApplied is true when some configured rate sits below the floor and is
	// therefore clamped. Reported so a misconfiguration is visible, not silent.
	floorApplied := false
	if tier.PoNAEnabled() {
		for _, band := range []types.ActivityBand{
			types.BandMax, types.BandMid, types.BandLow, types.BandSingle,
		} {
			if tier.RewardForBand(band).LT(tier.ZeroActivityReward) {
				floorApplied = true

				break
			}
		}
	}

	return tierRates{
		LowTxCountThreshold: tier.LowTxCountThreshold,
		MidTxCountThreshold: tier.MidTxCountThreshold,
		MaxTxCountThreshold: tier.MaxTxCountThreshold,
		PoNAEnabled:         tier.PoNAEnabled(),
		Max:                 newAmountReport(tier.MaxActivityReward),
		Mid:                 newAmountReport(tier.MidActivityReward),
		Low:                 newAmountReport(tier.LowActivityReward),
		Single:              newAmountReport(tier.SingleActivityReward),
		Zero:                newAmountReport(tier.ZeroActivityReward),
		FloorApplied:        floorApplied,
	}
}

// explainBand states in plain language why a band applies to a block, and which
// amount the tier actually pays for it (after the zero-activity floor).
func explainBand(tier types.Tier, txCount uint64) string {
	band := tier.BandForTxCount(txCount)
	paid := tier.RewardForTxCount(txCount)

	// floored describes the amount when the configured rate was clamped up.
	floored := func(configured sdkmath.Int) string {
		if paid.GT(configured) {
			return fmt.Sprintf(
				"the configured rate of %s RUNE is below the zero-activity floor, so %s RUNE is paid",
				runeString(configured), runeString(paid),
			)
		}

		return fmt.Sprintf("%s RUNE applies", runeString(paid))
	}

	switch band {
	case types.BandZero:
		return fmt.Sprintf(
			"the block contains no regular transactions, so the zero-activity rate of %s RUNE applies",
			runeString(paid),
		)
	case types.BandSingle:
		return fmt.Sprintf(
			"the block contains exactly 1 regular transaction, so the single-activity rate %s",
			floored(tier.SingleActivityReward),
		)
	case types.BandLow:
		return fmt.Sprintf(
			"%d regular transaction(s) is in the low band (%d..%d), so the low-activity rate %s",
			txCount, tier.LowTxCountThreshold, tier.MidTxCountThreshold-1, floored(tier.LowActivityReward),
		)
	case types.BandMid:
		return fmt.Sprintf(
			"%d regular transaction(s) is in the mid band (%d..%d), so the mid-activity rate %s",
			txCount, tier.MidTxCountThreshold, tier.MaxTxCountThreshold-1, floored(tier.MidActivityReward),
		)
	default:
		return fmt.Sprintf(
			"%d regular transaction(s) meets the max threshold of %d, so the max rate %s",
			txCount, tier.MaxTxCountThreshold, floored(tier.MaxActivityReward),
		)
	}
}

// tierEntry is one entry of `query pona params`.
type tierEntry struct {
	Index       int    `json:"index" yaml:"index"`
	StartHeight uint64 `json:"start_height" yaml:"start_height"`
	// EndHeight 0 means open-ended (the final tier).
	EndHeight      uint64 `json:"end_height" yaml:"end_height"`
	OpenEnded      bool   `json:"open_ended" yaml:"open_ended"`
	LengthInBlocks string `json:"length_in_blocks" yaml:"length_in_blocks"`
	LengthInTime   string `json:"length_in_time" yaml:"length_in_time"`
	tierRates      `yaml:",inline"`
}

// newTierEntry builds one schedule entry.
func newTierEntry(index int, tier types.Tier) tierEntry {
	entry := tierEntry{
		Index:          index,
		StartHeight:    tier.StartHeight,
		EndHeight:      tier.EndHeight,
		OpenEnded:      tier.EndHeight == 0,
		LengthInBlocks: "unbounded",
		LengthInTime:   "unbounded",
		tierRates:      newTierRates(tier),
	}

	if tier.EndHeight != 0 {
		length := tier.EndHeight - tier.StartHeight

		entry.LengthInBlocks = strconv.FormatUint(length, 10)
		entry.LengthInTime = durationFromBlocks(length)
	}

	return entry
}

// paramsReport is the output of `query pona params`.
type paramsReport struct {
	Height int64       `json:"height" yaml:"height"`
	Denom  string      `json:"denom" yaml:"denom"`
	Tiers  []tierEntry `json:"tiers" yaml:"tiers"`
}

// newParamsReport builds the params output.
func newParamsReport(params types.Params, height int64) paramsReport {
	tiers := make([]tierEntry, 0, len(params.Tiers))
	for i, tier := range params.Tiers {
		tiers = append(tiers, newTierEntry(i, tier))
	}

	return paramsReport{
		Height: height,
		Denom:  params.Denom,
		Tiers:  tiers,
	}
}

// poolReport is the output of `query pona pool`.
type poolReport struct {
	Height        int64  `json:"height" yaml:"height"`
	ModuleAccount string `json:"module_account" yaml:"module_account"`
	Address       string `json:"address" yaml:"address"`
	Denom         string `json:"denom" yaml:"denom"`
	Balance       string `json:"balance" yaml:"balance"`
	BalanceRune   string `json:"balance_rune" yaml:"balance_rune"`
	TierIndex     int    `json:"tier_index" yaml:"tier_index"`
	// MaxRewardPerBlock is the active tier's MAXIMUM rate, used for the estimate.
	MaxRewardPerBlock string `json:"max_reward_per_block,omitempty" yaml:"max_reward_per_block,omitempty"`
	MaxRewardRune     string `json:"max_reward_rune,omitempty" yaml:"max_reward_rune,omitempty"`
	// RemainingBlocks assumes every future block pays the MAX rate, which makes
	// it a lower bound on how long the reserve lasts.
	RemainingBlocks   string `json:"remaining_blocks" yaml:"remaining_blocks"`
	RemainingDuration string `json:"remaining_duration" yaml:"remaining_duration"`
	EmissionEnded     bool   `json:"emission_ended" yaml:"emission_ended"`
}

// scheduleView is the shared single-tier view used by the `tier`,
// `block-reward` and `tx-count` commands.
type scheduleView struct {
	TierIndex     int    `json:"tier_index" yaml:"tier_index"`
	TierCount     int    `json:"tier_count" yaml:"tier_count"`
	StartHeight   uint64 `json:"start_height" yaml:"start_height"`
	EndsAtHeight  uint64 `json:"ends_at_height" yaml:"ends_at_height"`
	EndsAtMeaning string `json:"ends_at_height_meaning" yaml:"ends_at_height_meaning"`
	tierRates     `yaml:",inline"`
}

// fillFromTier populates the shared single-tier fields from a tier.
func (v *scheduleView) fillFromTier(tier types.Tier) {
	v.StartHeight = tier.StartHeight
	v.EndsAtHeight = tier.EndHeight
	if tier.EndHeight == 0 {
		v.EndsAtMeaning = "open-ended (final tier)"
	} else {
		v.EndsAtMeaning = "exclusive"
	}
	v.tierRates = newTierRates(tier)
}

// tierReport is the output of `query pona tier`.
type tierReport struct {
	Height int64 `json:"height" yaml:"height"`
	// View holds the shared single-tier fields. It is a named field rather than
	// an embedded one so the promoted fields stay addressable in literals on
	// every supported Go version.
	View                   scheduleView `json:"tier" yaml:"tier"`
	NextTierStartHeight    uint64       `json:"next_tier_start_height,omitempty" yaml:"next_tier_start_height,omitempty"`
	BlocksUntilNextTier    string       `json:"blocks_until_next_tier" yaml:"blocks_until_next_tier"`
	TimeUntilNextTier      string       `json:"time_until_next_tier" yaml:"time_until_next_tier"`
	NextTierMaxReward      string       `json:"next_tier_max_reward,omitempty" yaml:"next_tier_max_reward,omitempty"`
	NextTierMaxRewardRune  string       `json:"next_tier_max_reward_rune,omitempty" yaml:"next_tier_max_reward_rune,omitempty"`
	NextTierMidReward      string       `json:"next_tier_mid_reward,omitempty" yaml:"next_tier_mid_reward,omitempty"`
	NextTierMidRewardRune  string       `json:"next_tier_mid_reward_rune,omitempty" yaml:"next_tier_mid_reward_rune,omitempty"`
	NextTierLowReward      string       `json:"next_tier_low_reward,omitempty" yaml:"next_tier_low_reward,omitempty"`
	NextTierLowRewardRune  string       `json:"next_tier_low_reward_rune,omitempty" yaml:"next_tier_low_reward_rune,omitempty"`
	NextTierZeroReward     string       `json:"next_tier_zero_reward,omitempty" yaml:"next_tier_zero_reward,omitempty"`
	NextTierZeroRewardRune string       `json:"next_tier_zero_reward_rune,omitempty" yaml:"next_tier_zero_reward_rune,omitempty"`
}

// blockRewardReport is the output of `query pona block-reward`.
type blockRewardReport struct {
	Height int64 `json:"height" yaml:"height"`
	// TxCount is the number of regular transactions the reward was computed for.
	TxCount uint64 `json:"tx_count" yaml:"tx_count"`
	// TxCountSource says where that count came from.
	TxCountSource string `json:"tx_count_source" yaml:"tx_count_source"`
	// Band is the PoNA band in effect: max, mid, low, single, zero or none.
	Band string `json:"band" yaml:"band"`
	// Reward is the amount this block pays.
	Reward     string `json:"reward" yaml:"reward"`
	RewardRune string `json:"reward_rune" yaml:"reward_rune"`
	Denom      string `json:"denom" yaml:"denom"`
	// BandExplanation states in plain language why this band was selected.
	BandExplanation string `json:"band_explanation" yaml:"band_explanation"`
	// View holds the shared single-tier fields for the active tier.
	View scheduleView `json:"tier" yaml:"tier"`
	// Pool context: can the reserve actually pay this block?
	PoolBalance     string `json:"pool_balance,omitempty" yaml:"pool_balance,omitempty"`
	PoolBalanceRune string `json:"pool_balance_rune,omitempty" yaml:"pool_balance_rune,omitempty"`
	PoolSufficient  bool   `json:"pool_sufficient" yaml:"pool_sufficient"`
	PoolError       string `json:"pool_error,omitempty" yaml:"pool_error,omitempty"`
}

// txCountReport is the output of `query pona tx-count`.
type txCountReport struct {
	Height int64 `json:"height" yaml:"height"`
	// TxCount is the number of REGULAR transactions in the block: what PoNA
	// counted, and what selected the band.
	TxCount uint64 `json:"tx_count" yaml:"tx_count"`
	// BlockTxCount is every entry physically in the block, including the injected
	// pseudo-transaction. It equals what CometBFT logs as num_txs.
	BlockTxCount        uint64 `json:"block_tx_count" yaml:"block_tx_count"`
	BlockTxCountNote    string `json:"block_tx_count_note,omitempty" yaml:"block_tx_count_note,omitempty"`
	BlockTxCountError   string `json:"block_tx_count_error,omitempty" yaml:"block_tx_count_error,omitempty"`
	Band                string `json:"band" yaml:"band"`
	TierIndex           int    `json:"tier_index" yaml:"tier_index"`
	LowTxCountThreshold uint64 `json:"low_tx_count_threshold" yaml:"low_tx_count_threshold"`
	MidTxCountThreshold uint64 `json:"mid_tx_count_threshold" yaml:"mid_tx_count_threshold"`
	MaxTxCountThreshold uint64 `json:"max_tx_count_threshold" yaml:"max_tx_count_threshold"`
	Reward              string `json:"reward" yaml:"reward"`
	RewardRune          string `json:"reward_rune" yaml:"reward_rune"`
	TxCountNote         string `json:"note" yaml:"note"`
}
