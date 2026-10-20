# Node Setup

> **Unique to Hoodium.** This is the guided, first-run setup experience built into
> the `runed` binary. Most Cosmos/EVM chains require you to wire `init` and
> `keys add` together by hand; Hoodium gives you a single guided entry point with
> an **automated** and a **manual** mode.

## The two modes

Running `runed setup` walks you through the whole bootstrap and asks you to pick a
mode:

| Mode | What it does |
|------|--------------|
| **Automated** | Asks for confirmation, then **runs** `init` and `keys add` for you (with prompts). |
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

## ✅ What `runed setup` does and does not do

`runed setup` prepares the **node files** — `init` (genesis placeholder + config +
node keys) and your **wallet key**.

| Mode | What it runs | Good for |
|---|---|---|
| **Manual** | *Prints* `init` + `keys add` for you to run yourself | Either case — you stay in control |
| **Automated** | *Runs* `init` + `keys add` for you | Faster bootstrap |

**In both modes, you still must:** replace the placeholder `genesis.json` with the
**official** one, set your peers, fund your wallet, sync, and then
**`create-validator`** — see [Validator Setup](./validator.md).

## Next steps

- **Joining an official network as a validator** → [Validator Setup](./validator.md)
  (full guide: init → official genesis → peers → wallet → sync → `create-validator`).
