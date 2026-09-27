package app

import (
	"fmt"

	upgradetypes "cosmossdk.io/x/upgrade/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NOTE: Historical Hoodium upgrade and fork handlers have been removed as part of
// the rebrand to a fresh chain (no prior state to migrate). The upgrade module
// itself is kept so future chain-software upgrades can be scheduled. To add an
// upgrade later, define an upgrade handler, register it via
// app.UpgradeKeeper.SetUpgradeHandler, and populate the Upgrades slice here.

var (
	// Upgrades holds the named upgrade handlers for the chain. Empty for a
	// fresh chain; populate when scheduling the first software upgrade.
	Upgrades = []UpgradeHandler{}

	// Forks holds the height-gated fork handlers for the chain. Empty for a
	// fresh chain.
	Forks = []ForkHandler{}
)

// UpgradeHandler is a minimal placeholder describing a named software upgrade
// handler. Replace with the real upgrade type when upgrades are reintroduced.
type UpgradeHandler struct {
	Name string
}

// ForkHandler is a minimal placeholder describing a height-gated fork handler.
type ForkHandler struct{}

// BeginBlockForks is intended to be run in a chain upgrade.
func (app *Hoodium) beginBlockForks(_ sdk.Context) {
	// No forks registered for a fresh chain.
}

func (app *Hoodium) setupUpgradeHandlers() {
	// No upgrade handlers registered for a fresh chain.
	app.setupUpgradeStoreLoaders()
}

func (app *Hoodium) setupUpgradeStoreLoaders() {
	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(fmt.Sprintf("failed to read upgrade info from disk %s", err))
	}

	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}

	currentHeight := app.CommitMultiStore().LastCommitID().Version

	if upgradeInfo.Height == currentHeight+1 {
		app.customPreUpgradeHandler(upgradeInfo)
	}
}

//nolint:all
func (app *Hoodium) customPreUpgradeHandler(upgradeInfo upgradetypes.Plan) {
	switch upgradeInfo.Name {
	default:
		// no-op
		return
	}
}
