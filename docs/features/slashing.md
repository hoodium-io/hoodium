## Slashing

Hoodium wires the Cosmos **`x/slashing`** module. It provides the network's
**economic security**: a validator that misbehaves loses stake, not just rewards.

> **Wired 2026-10-11** — `x/slashing` keeper, store key, module, `BeginBlock`
> ordering and the staking hooks. `x/evidence` (external double-sign reporting)
> is **deferred**; see below.

### Parameters (set explicitly in genesis)

| Parameter | Hoodium | SDK default | Why |
|---|---|---|---|
| `SlashFractionDoubleSign` | **10%** | 5% | Equivocation is the one unrecoverable fault — penalty doubled. |
| `SlashFractionDowntime` | **1%** | 1% | Kept small; the real deterrent for downtime is the jail. |
| `DowntimeJailDuration` | **24 hours** | 10 min | Long enough to matter, short enough that a failed restart is recoverable. |
| `SignedBlocksWindow` | **10,000 blocks** | 100 | ≈16.7 h at 6 s blocks. The default (~10 min) is unusable on a real chain. **Per validator** — each tracks its own signed-block window. |
| `MinSignedPerWindow` | **50%** | 50% | ≈8.3 h of downtime tolerated before jailing. Tighter than 25%, giving a stronger liveness guarantee. |

> The official `genesis.json` is authoritative — these values are baked in and
> are **not** meant to be edited per network.

> 💡 **`SignedBlocksWindow` is per validator, not network-wide.** Each validator
> keeps its own rolling window and signed-block record; jailing is decided
> individually. One validator being offline does not affect another's counter.

### Where do slashed funds go? — 🔥 **They are burned**

This is the SDK behaviour and Hoodium keeps it. `x/staking/keeper/slash.go`
removes the tokens from the bonded/not-bonded pool and calls `BurnCoins`:

```go
// Burn the slashed tokens from the pool account and decrease the total supply.
k.burnBondedTokens(ctx, tokensToBurn)   // or burnNotBondedTokens
```

| | Effect |
|---|---|
| Destination | 🔥 Burned — **nobody receives them** |
| Supply | **Decreases** (deflationary) |
| Community pool / reward pool | **Nothing** is sent there |

> ⚠️ **Consequence for the 10B cap:** the cap is a *ceiling*, not a guarantee.
> Every slash permanently lowers total supply below 10B. The "9B premine + 1B
> reserve = exactly 10B" identity only holds at genesis.

### Downtime vs double-sign

| Fault | Detected by | Penalty |
|---|---|---|
| **Downtime** (missing blocks) | `x/slashing` itself, from signed-block tracking | 1% slash + jailed 24h |
| **Double-sign** (equivocation) | Needs **evidence** to be reported | 10% slash + tombstoned |

### Deferred: `x/evidence`

`x/slashing` does **not** depend on `x/evidence` — it handles downtime entirely on
its own. `x/evidence` adds the mechanism for **submitting** double-sign evidence.

**Deferred deliberately:** a devnet never equivocates, so there is nothing to test,
and wiring it now would be untestable plumbing. **Tracked as a pre-mainnet item.**
Until it is wired, double-sign evidence cannot be reported (the slash *logic* is
present, but nothing feeds it).
