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

### Where do slashed funds go? — **Redirected to the fee pool (nothing is burned)**

This is a **Hoodium customisation**. The SDK default burns slashed tokens; Hoodium
does not.

**How it works** — `app/slashingAdapter.go` (`runeStakingKeeper`):

```
1. Real staking slash runs  → tokens burnt by x/staking (SDK behaviour)
2. Wrapper re-mints the SAME amount
3. Sends it to the distribution FEE COLLECTOR
4. x/distribution sweeps the fee collector every block and
   splits it across the ACTIVE validator set by voting power
```

| | Effect |
|---|---|
| Destination | **Fee pool** → active validator set, proportional to voting power |
| Supply | **Unchanged** — burn −X then mint +X = **0** |
| 10B cap | Never exceeded (the re-mint is bounded by the amount slashed) |

**Why not burn?** Burning would make the 10B cap a ceiling that supply drifts
below, and it destroys value rather than recycling it.

**Why the fee pool and not `validator_reward_pool`?**
- Fee-collector funds reach the **active validator set every block** — a live,
  network-wide payout that suits a network-level offence.
- It does **not extend** the 1B validator reward emission schedule.
- It matches Hoodium's **end state**, where rewards are fees-only.

**Why no fork was needed:** `x/slashing` does not call the concrete staking
keeper — it calls the `StakingKeeper` **interface**
(`x/slashing/types/expected_keepers.go`). Hoodium supplies its own implementation
(`runeStakingKeeper`) that wraps the real keeper and overrides only the two
`Slash*` methods. Every other method is the standard one.

**Abuse resistance:** the re-mint is exactly bounded by what the real keeper
reports as slashed. An attacker cannot inflate supply — causing a slash requires
an offence that costs them more than they gain.

> ✅ **Mints from the `runerewards` module account**, which already holds the
> `Minter` permission (`maccPerms` in `app/app.go`). No new privileges are granted
> to `staking` — its authority is unchanged from stock Cosmos.

### Downtime vs double-sign

| Fault | Detected by | Penalty |
|---|---|---|
| **Downtime** (missing blocks) | `x/slashing` itself, from signed-block tracking | 1% slash + jailed 24h |
| **Double-sign** (equivocation) | Needs **evidence** to be reported | 10% slash + tombstoned |

In both cases the slashed amount is **redirected to the fee pool**, not burned.

### Deferred: `x/evidence`

`x/slashing` does **not** depend on `x/evidence` — it handles downtime entirely on
its own. `x/evidence` adds the mechanism for **submitting** double-sign evidence.

**Deferred deliberately:** a devnet never equivocates, so there is nothing to test,
and wiring it now would be untestable plumbing. **Tracked as a pre-mainnet item.**
Until it is wired, double-sign evidence cannot be reported (the slash *logic* is
present, but nothing feeds it).
