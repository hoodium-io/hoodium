# Upgrades

This document describes how chain-software upgrades will be scheduled on
Hoodium.

> **Fresh chain:** Hoodium has no prior chain state to migrate, so there are
> **no upgrade handlers or forks registered yet**. See
> [`app/upgrades.go`](../app/upgrades.go).

## The upgrade module

Hoodium uses the Cosmos SDK **`x/upgrade`** module. It is wired into the app so
that future software upgrades can be scheduled without changing the chain's
architecture. Two primitives are available:

- **Upgrade handler** — a named handler that runs state migrations when the
  chain halts at a scheduled height. Used for store-schema or complex logic
  changes.
- **Height-gated fork** — a one-time code block executed at a programmatic
  height, for changes that do not need a coordinated halt.

Both are currently **empty** for Hoodium:

```go
// app/upgrades.go
var (
    Upgrades = []UpgradeHandler{}  // no upgrades registered yet
    Forks    = []ForkHandler{}     // no forks registered yet
)
```

## Adding an upgrade later

1. Define the state-migration logic for the target consensus version.
2. Register an upgrade handler via `app.UpgradeKeeper.SetUpgradeHandler`.
3. Populate the `Upgrades` (or `Forks`) slice in `app/upgrades.go`.
4. Schedule the upgrade (a governance `MsgSoftwareUpgrade`, or a height-gated
   fork).

Store migrations use the Cosmos SDK in-place store migration mechanism.

## Module consensus versions

Each module carries a `ConsensusVersion`. Bumping one signals that the module's
on-disk state changed, and a migration must be registered for it:

| Module | ConsensusVersion |
|---|---|
| `x/evm` | 1 |
| `x/feemarket` | 1 |
| `x/pona` | **3** |

> ⚠️ `x/pona` is at version **3**. History:
>
> - **1 → 2** — gained the PoNA activity fields and dropped `min_reward_per_block`.
> - **2 → 3** — `Tier` was restructured into the graded activity ladder:
>   `reward_per_block` → `max_activity_reward`, `tx_count_threshold` →
>   `max_tx_count_threshold`, plus three new fields (`mid_activity_reward`,
>   `low_tx_count_threshold`, `mid_tx_count_threshold`).
>
> A chain started on older state expects the matching migration. **Generate
> genesis fresh** (or register a migration) before booting from an old home
> directory. During devnet this is a non-issue — wipe and regenerate.