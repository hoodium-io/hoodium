# Mainchain Genesis Initialization (TEAM ONLY)

> **⚠️ TEAM USE ONLY.** This document is for the **Hoodium team** and describes the
> command used to generate the **official Hoodium Mainchain genesis** at network
> launch (devnet, testnet or mainnet).
>
> **If you are a validator or node operator who wants to join an already-running
> network, this is NOT the document for you** — see
> [Node Setup](../../node-setup.md) and [Validator Setup](../../validator.md).

## The command

```
runed genesis init-files --val <N> \
  --output-dir <dir> \
  --chain-id <chain-id> \
  --starting-ip-address <ip>
```

This is the **official genesis generator**. Running it produces the complete,
production-shaped genesis for a new Hoodium network, including:

- The **RUNE premine** → the genesis validator's wallet
- The **1B RUNE validator reward reserve** → `validator_reward_pool`
- The **bonded tokens pool** funding
- **Staking** parameters (max validators, min self-delegation, commission)
- **EVM** and **fee-market** parameters
- Per-node config files (`config.toml`, `app.toml`, `client.toml`), consensus keys
  and P2P keys

## Why it lives under `runed genesis`

It is deliberately **not** exposed as `runed testnet …`, `runed mainnet …` or
`runed devnet …`. Generating a network's genesis is a **one-time team action**,
not something a joining validator ever does. Putting it under `runed genesis`
keeps it clearly separate from the way operators join a network
(`runed setup`), so no one mistakes "make a mainnet genesis" for
"join mainnet".

The network is selected entirely by `--chain-id`:

| Network | Chain ID |
|---|---|
| Mainnet | `rune_6590-1` |
| Testnet | `rune_6591-1` |
| Devnet  | `rune_6592-1` |

## What the bootstrap sequence does

```
runed init <moniker> --chain-id <chain-id> --home <home>
runed keys add <key-name> --keyring-backend file --home <home>
runed genesis gentx <key-name> <amount> --pubkey "$(runed tendermint show-validator)" \
  --chain-id <chain-id> --home <home>
runed start --home <home>
```

- `init` writes `genesis.json`, `app.toml`, `client.toml`, `config.toml` and the
  node's **consensus key** (`priv_validator_key.json`) and **p2p key**.
- `keys add` creates your **wallet key** (eth_secp256k1). This account holds RUNE
  and, for the bootstrap validator, receives the genesis premine.
- `gentx` records your intent to become a validator; it is collected into genesis
  on a fresh network, or replaced by `tx staking create-validator` on a live one.

## Bootstrap validator & the genesis premine

A single genesis validator is enough to **boot** the network. The genesis premine
is allocated to the **bootstrap validator's wallet** (the account created by
`keys add`):

| Token | Amount | Destination |
|-------|--------|-------------|
| RUNE  | 9,000,000,000 | bootstrap validator wallet (its 500,000 RUNE self-delegation is bonded from this) |
| RUNE  | 1,000,000,000 | `validator_reward_pool` (funds block rewards, `x/runerewards`) |
| HOODI | 10,000,000    | bootstrap validator wallet |

RUNE total = **10,000,000,000** (cap). HOODI total = **10,000,000** (cap).

The premine destinations are **derived at runtime** — there are no hardcoded
addresses to edit and no recompile:

- The premine lands on the **first account created by `keys add`** in the
  command's loop, i.e. the operator running this command controls it.
- `validator_reward_pool` is a Cosmos **module account**: its address is always
  `ripemd160(sha256("validator_reward_pool"))` =
  `0x4967715BC2Ba5CC2A262054A5a893e2E90FCD3f9`
  (bech32 `rune1f9nhzk7zhfwv9gnzq4994zf796g0e5lej732lk`).

### Accessing the premine from an EVM wallet

A Hoodium account has a single balance reachable through **two encodings of the
same 20 bytes**:

- **`0x...`** — the EVM address (MetaMask, etc.)
- **`rune1...`** — the Cosmos/bech32 address

To move the premine into an EVM wallet, export the wallet's private key as hex
and import it:

```
runed keys unsafe-export-eth-key <key-name> --keyring-backend file --home <home>
```

Paste the resulting `0x` private key into MetaMask (import account). The premine
balance is then spendable from the EVM side immediately.

> ⚠️ `unsafe-export-eth-key` prints an **unencrypted** private key. Treat it like
> cash; never share it or paste it into an untrusted tool.

## Publishing the network

Once the genesis is generated:

1. Distribute the **official `genesis.json`** — its hash identifies the network.
2. Publish the **seed / persistent peer** addresses.
3. Validators then join using [Node Setup](../../node-setup.md) and
   [Validator Setup](../../validator.md).

Only a validator's **public consensus key** is recorded in genesis
(`validators[]`); the private `priv_validator_key.json` never leaves the node.

## Staking parameters (generated)

| Parameter | Value |
|-----------|-------|
| Max validators | 101 |
| Min self-delegation | 500,000 RUNE |
| Min commission rate | 5% |

> A minimum **delegation** of 0.1 RUNE is enforced at the message/precompile level
> (not a staking param).