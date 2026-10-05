# Validator & Node Setup

> **Unique to Hoodium.** This is the guided, first-run setup experience built into
> the `runed` binary. Most Cosmos/EVM chains require you to wire `init`, `keys add`
> and `gentx` together by hand; Hoodium gives you a single guided entry point with
> an **automated** and a **manual** mode.

## The two modes

Running `runed setup` walks you through the whole bootstrap and asks you to pick a
mode:

| Mode | What it does |
|------|--------------|
| **Automated** | Asks for confirmation, then **runs** `init`, `keys add` and `gentx` for you (with prompts). |
| **Manual** | **Prints** the exact copy-paste commands so you stay in full control. |

You can skip the interactive prompts by passing flags (see below).

## Guided flow

`runed setup` asks, in order:

1. **Mode** — `automated` or `manual`
2. **Network** — `mainnet`, `testnet` or `devnet`
3. **Node type** — `validator` or `seed` (non-validating)
4. **Moniker** — a name for your node
5. **Wallet key name** — (validator only) the account that holds/stakes RUNE

The network choice maps to the chain-id automatically:

| Network | Chain ID |
|---------|----------|
| Mainnet | `rune_6590-1` |
| Testnet | `rune_6591-1` |
| Devnet  | `rune_6592-1` |

> **Devnet is a full mainnet replica** used to accelerate testing. It is *not* a
> localnet — it runs the same production configuration, real consensus and the
> production genesis shape.

## Non-interactive usage

```
runed setup \
  --mode manual \
  --network testnet \
  --node-type validator \
  --moniker my-node \
  --key-name my-validator-key
```

Flags: `--mode`, `--network`, `--node-type`, `--moniker`, `--key-name`,
`--stake-amount`, `--home`, `--chain-id`, `--keyring-backend`.

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

### Accessing the premine from an EVM wallet

A Hoodium account has a single balance that is reachable through **two encodings
of the same 20 bytes**:

- **`0x...`** — the EVM address (MetaMask, etc.)
- **`rune1...`** — the Cosmos/bech32 address

They are the **same account**. To move the premine into an EVM wallet, export the
wallet's private key as hex and import it:

```
runed keys unsafe-export-eth-key <key-name> --keyring-backend file --home <home>
```

Paste the resulting `0x` private key into MetaMask (import account). The premine
balance is then spendable from the EVM side immediately.

> ⚠️ `unsafe-export-eth-key` prints an **unencrypted** private key. Treat it like
> cash; never share it or paste it into an untrusted tool.

## Joining an already-running network

Additional validators join **later, from their own nodes**, without any action from
the bootstrap operator:

1. They obtain the **official `genesis.json`** (same file everyone uses — its hash
   identifies the network).
2. They run `init` with the **same chain-id** and the shared genesis.
3. They fund their own wallet and submit:
   ```
   runed tx staking create-validator \
     --amount 500000000000000000000000arune \
     --pubkey "$(runed tendermint show-validator)" \
     --moniker <moniker> --commission-rate 0.05 \
     --min-self-delegation 500000000000000000000000 \
     --from <key-name> --chain-id <chain-id> --home <home>
   ```

Only the validator's **public consensus key** is recorded in genesis (`validators[]`);
the private `priv_validator_key.json` never leaves the node.

The bootstrap validator has **no special role** once other validators are bonded:
if it goes down, the remaining validators keep the chain running (provided less than
one third of the voting power is offline).

## Staking parameters (Hoodium)

| Parameter | Value |
|-----------|-------|
| Max validators | 101 |
| Min self-delegation | 500,000 RUNE |
| Min commission rate | 5% |

> A minimum **delegation** of 0.1 RUNE is enforced at the message/precompile level
> (not a staking param).