# Hoodium Network

**The Unified Dual-Layer Blockchain Network built on top of Bitcoin** — *in active development.*

A combination of **L1 (Mainchain EVM)** + **L2 (Sidechain POX)**.

> 🚧 **Development status: Mainchain Devnet.**
> Hoodium is building toward **Mainchain Mainnet launch**. A public **Mainchain devnet**
> will be live so the community can watch the network evolve in real development —
> from devnet, through testnet, to Mainnet final. The **YieldChain (L2 / POX)** is a
> separate track and is **not** part of the Mainchain devnet.

---

## Roadmap

Progress toward **Mainchain Mainnet**. Items are checked as they land.

### Phase 1 — Foundation
- [x] Codebase forked and rebranded to `hoodium-io/hoodium` (module path, binary `runed`, home `~/.runed`)
- [x] Bridge / tBTC / sidecar / pseudo-tx removal (out-of-scope legacy surface)
- [x] Chain identity: chain-IDs `6590`/`6591`, denom `arune`, precompile prefix
- [x] `x/poa` removed → **standard Cosmos staking** (101 validators, 500,000 RUNE self-delegation, 5% commission)
- [x] EVM (`x/evm`) + fee market (`x/feemarket`) wired and green
- [x] Two-gas-price model, governance-adjustable
- [x] RUNE genesis allocation: **9B premine + 1B reward reserve = 10B cap**
- [x] `make install` + `make test-unit` green

### Phase 2 — RUNE rewards (this cycle)
- [x] `x/runerewards` module — 1B validator reward pool, tiered emission schedule
- [x] Emission schedule: **50 / 25 / 10 RUNE per block** (Y0–2 / Y2–4 / Y4+)
- [x] **PoNA (Proof of Network Activity)** — block reward scaled by transactions per block
- [x] Fee-only fallback when the reserve is exhausted
- [ ] Validator reward payout routed through `x/distribution` commission split

### Phase 3 — Devnet launch
- [x] Devnet boots end-to-end (genesis, staking, EVM, rewards)
- [x] RUNE token precompile (gas / gov / reward token)
- [ ] **Public Mainchain devnet** — genesis, seeds, faucet, explorer, docs
- [ ] Public devnet monitoring + upgrade process

### Phase 4 — Mainchain feature build-out
- [ ] HOODI token precompile (Mainchain-side)
- [ ] Governance module wiring (RUNE as gov token)
- [ ] `RUNE/USD` price oracle (pricefeeder)
- [ ] Blockshield — dust-spam protection + airdrop guard
- [ ] Min-delegation (0.1 RUNE) enforcement at msg/precompile level
- [ ] Hardhat ABI regeneration for all precompiles

### Phase 5 — Mainchain Mainnet
- [ ] External audits
- [ ] Mainnet genesis ceremony
- [ ] Mainnet launch

### Phase 6 — YieldChain (L2 / POX) — *separate track*
- [ ] POX Yieldchain engine
- [ ] Bitcoin-hash secured consensus
- [ ] Dual-chain bridge

---

## Features

Custom features built for Hoodium (i.e. beyond stock Cosmos SDK / EVM behaviour).

| Feature | Description |
|---|---|
| **RUNE native token** | The single token for gas, governance and validator/delegator rewards on the Mainchain. Denom `arune` (1 RUNE = 10^18 arune); total supply capped at **10B RUNE**. |
| **Fixed-supply emission** | Block rewards come from a **pre-funded 1B RUNE reserve**, not Cosmos-style inflation. When the reserve empties, the chain runs **fee-only** permanently. |
| **Tiered emission schedule** | Static reward steps down over time: **50 RUNE/block** (Y0–2) → **25** (Y2–4) → **10** (Y4+). |
| **PoNA — Proof of Network Activity** | The block reward scales with how many transactions the block actually contains: full rate at `tx >= 10`, a reduced rate at `1-9` tx, and a nominal **1 RUNE** for an empty block. Idle chains drain the reserve far more slowly. |
| **Two-gas-price model** | Separate minimum gas prices for **regular** vs **deployment** transactions, both **governance-adjustable**. |
| **Standard staking** (replacing PoA) | Permissionless Cosmos staking: **101** max validators, **500,000 RUNE** min self-delegation, **5%** min commission. |
| **Staking from EVM** | A staking precompile lets EVM accounts delegate / undelegate / query without leaving the EVM toolchain. |
| **Dual-chain architecture** | **L1 Mainchain (EVM)** + **L2 YieldChain (POX)** as one unified network. *(YieldChain is a separate, later track.)* |

## Technologies

Custom technologies and protocol machinery built so far. Mechanism-level details live
in [`docs/technologies/`](./docs/technologies/README.md).

| Technology | Description | Doc |
|---|---|---|
| **`x/runerewards`** | Custom module owning the 1B RUNE validator reward pool and the emission schedule: funds the reserve, computes the block reward, and pays the proposer. | [`runerewards.md`](./docs/technologies/runerewards.md) |
| **PoNA (Proof of Network Activity)** | Activity-scaled reward engine. Captures the block's transaction count in the app's `PreBlock` hook, stores it, and selects the tier's full / low / zero rate in `EndBlock`. Pure and deterministic. | [`pona.md`](./docs/technologies/pona.md) |
| **EVM precompiles** (5 registered) | Native Go precompiles exposed at fixed addresses: **RUNE token** `0x19be...0000`, **HOODI token** `0x19be...0001`, **PriceOracle** `0x19be...0002`, **Staking** `0x19be...0003`, **TestBed** `0x19BE10...0000`. | [`precompile.md`](./docs/precompile.md) |
| **Two-gas-price model** | `x/feemarket` extension adding `MinRegularGasPrice` / `MinDeploymentGasPrice` on top of the EIP-1559 base fee. | [`tx-fees.md`](./docs/features/tx-fees.md) |
| **EVM + fee market** | Full EVM execution (`x/evm`, forked from Cosmos EVM) with a working EIP-1559 fee market (`x/feemarket`). | [`evm-compatibility.md`](./docs/evm-compatibility.md) |
| **Custom chain identity** | Chain-IDs `6590`/`6591`, `arune` denom, `rune` bech32 prefix, precompile address space prefixed `0x19be` (derived from the chain ID). | — |
| **RUNE genesis model** | 9B premine + 1B reward reserve, both derived at runtime — no hardcoded addresses. | [`testnet.md`](./docs/testnet.md) |

## Overview

Hoodium is a unified dual-layer network:

- **L1 — Mainchain:** an EVM-compatible application chain (Cosmos SDK) secured by its own
  validator set. RUNE is the gas, governance and reward token. This is what the devnet runs.
- **L2 — YieldChain (POX):** a sidechain targetting Bitcoin-mining-hash security. *Separate,
  later track — not part of the Mainchain devnet.*

Both layers share one network identity. Development proceeds Mainchain-first: the Mainchain
must be solid before the YieldChain is layered on.

## Documentation

Full documentation lives in [`docs/`](./docs/):

| Doc | What it covers |
|---|---|
| [Technologies](./docs/technologies/README.md) | How Hoodium's custom mechanisms work (PoNA, rewards, …) |
| [Features](./docs/features/README.md) | User-facing Hoodium features |
| [Development](./docs/development.md) | Running the client locally |
| [Validator setup](./docs/validator-setup.md) | `runed setup` and joining as a validator |
| [Testnet](./docs/testnet.md) | The public test/dev network |
| [Precompile](./docs/precompile.md) | The EVM precompile set |

Also see the [documentation site](https://hoodium.dev/).

## Development

The Hoodium client can be run locally for development purposes. To learn more,
please consult the [development](./docs/development.md) guide.

## Contributing

Please consult our [contribution guide](./CONTRIBUTING.md) if you are willing
to contribute to the codebase.

## Security

The project has the [security policy](./SECURITY.md) available.

## Testnet

The Hoodium testnet is a public network that can be used for testing and
experimentation. To learn more, please consult the [testnet](./docs/testnet.md)
documentation.
---
