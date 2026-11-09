# Validator Setup

How to join an **official, already-running** Hoodium network as a validator.

> Running a node for the first time? Start with [Node Setup](./node-setup.md),
> which covers `runed setup` and its two modes.

## ✅ There is only ONE path — `create-validator`

Joining a Hoodium network is always the same action: **run a node against the
official genesis, then broadcast a `create-validator` transaction.**

## Joining a live network — full guide

### Step 0 — get the network artifacts from the Hoodium team

You need three things:

1. **`genesis.json`** — the official genesis file. Its hash identifies the
   network.
2. The **chain ID** (e.g. `rune_6592-1`).
3. The **seed / persistent peer** addresses.

**Get the official genesis** by cloning the chains repository:

```bash
git clone https://github.com/hoodium-io/chains.git
```

The Mainchain genesis lives at:

```
chains/mainchain/genesis.json
```

> The `hoodium-io/chains` repository publishes the canonical genesis file per
> network. **Never** use the default genesis that `runed init` generates — it has
> no validators, and will not connect you to the network.

### Step 1 — initialise the node with the official genesis

```bash
runed setup
```

`runed setup` initialises the node home (genesis placeholder, config, node keys),
creates your wallet key, and writes `validator.json` for you. Answer the prompts:

| Prompt | Answer |
|---|---|
| Mode | `m` (manual) or `a` (automated) |
| Network | `mainnet` · `testnet` · `devnet` |
| Node type | `v` (validator) |
| Moniker | **Type your own** — a short, recognisable node name. Do **not** accept the machine hostname: on a VPS that is often long and meaningless (e.g. `ip-10-0-1-23.eu-west-1.compute.internal`). |
| Wallet key name | Press **Enter** to accept your moniker, or type a different key name |

> ⚠️ **The moniker is your node's public display name** and it is written into
> `config.toml` and `validator.json`. It is *not* the wallet key name, though
> reusing the same name for both is the recommended convention.

Then **replace** the generated default genesis with the official one:

```bash
git clone https://github.com/hoodium-io/chains
cp chains/mainchain/<network>/genesis.json ~/.runed/config/genesis.json
```

`<network>` is the one you selected (`mainnet`, `testnet` or `devnet`).

**Verify the genesis hash** against the value published by the team before
continuing — if it does not match, you are on the wrong network.

### What you need after `runed setup`

`runed setup` prepares the **node files** (`init` + your wallet key). It does
**not** make you a live validator. You still must, in order:

1. Replace the placeholder `genesis.json` with the **official** one
2. Set the published seeds / `persistent_peers`
3. **Fund your wallet** (≥ 500,001 RUNE): <br>
Your account has ONE balance reachable as both its 0x and rune1 
address. Export the key once and import it into an EVM wallet,
then send RUNE to that address from anywhere - cmd:
```
runed keys unsafe-export-eth-key test-node \
--keyring-backend file --home /home/muroko/.runed
```
4. Start the node and let it **fully sync**
5. Broadcast **`create-validator`**

That is Steps 1–7 above. There is **no gentx** anywhere in this flow.

### Step 2 — configure peers

Add the seeds/persistent peers the team published, in `~/.runed/config/config.toml`.
They are the raw peer list for your network, published in the
`hoodium-io/chains` repository:

| Network | Seeds file |
|---|---|
| **devnet** | https://github.com/hoodium-io/chains/mainchain/devnet/seeds.txt |
| **testnet** | https://github.com/hoodium-io/chains/mainchain/testnet/seeds.txt |
| **mainnet** | https://github.com/hoodium-io/chains/mainchain/mainnet/seeds.txt |

```toml
persistent_peers = "<nodeID>@<host>:26656,..."
seeds = "<nodeID>@<host>:26656,..."
```

### Step 3 — create (or restore) your wallet key

```bash
runed keys add <key-name> --keyring-backend file --home ~/.runed
# or, to restore an existing key:
runed keys add <key-name> --recover --keyring-backend file --home ~/.runed
```

**Back up the mnemonic.** This account funds your self-delegation.

### Step 4 — fund the wallet

Your wallet needs at least **500,000 RUNE** for the self-delegation (plus gas).
Acquire RUNE from DEXs, CEXs etc...

```bash
runed query bank balances <your-rune1-address> --home ~/.runed
```

### Step 5 — start the node and let it sync

```bash
runed start --home ~/.runed
```

Watch it catch up:

```bash
runed status --home ~/.runed        # look at latest_block_height / catching_up
```

**Let it fully sync before creating the validator** — otherwise your
`create-validator` tx may fail or be created against a stale view.

### Step 6 — create the validator

`create-validator` takes a **JSON file**, not inline flags. `runed setup` already
generated it for you at **`~/.runed/validator.json`**, containing your consensus
pubkey and the terms below. Edit it first if you want a different moniker or amount.

| Field | Meaning |
|---|---|
| `pubkey` | Your consensus key, taken from `config/priv_validator_key.json` |
| `amount` | Self-delegation, in **base denom** (`arune`). Minimum 500,000 RUNE. |
| `moniker` | Your validator's display name |
| `commission-rate` | Initial commission — must be **≥ 5%** (chain minimum) |
| `commission-max-rate` | Upper bound the rate can ever reach |
| `commission-max-change-rate` | Largest single rate change |
| `min-self-delegation` | Minimum self-bond; must be ≤ `amount` |

Optional fields: `identity`, `website`, `security`, `details`.

**Submit it** (in a second terminal, while the node runs and is synced):

```bash
runed tx staking create-validator validator.json \
  --from <key-name> \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune \
  -y
```

> **`--home`, `--keyring-backend` and `--chain-id` are omitted on purpose.** All three
> are already in `~/.runed/config/client.toml` (written when you ran `runed init`), and
> every CLI command falls back to it. Passing `--chain-id` **overrides** that file: if
> the value does not match your node you get
> `signature verification failed … please verify chain-id (rune_…-1)` and the tx never
> reaches a block. Copy-pasting `rune_6590-1` (mainnet) onto a devnet/testnet node is
> the usual cause — let the config file supply it instead.

> **Why `--gas-prices` and not `--fees`?** The chain rejects any transaction whose
> fee is below `min_gas_price × gas`. With `--gas auto` the SDK first *simulates* the
> gas limit, then multiplies it by `--gas-prices` — so the fee always clears the floor
> and you never have to guess the gas. A fixed `--fees` amount cannot do that, and any
> amount below the floor is rejected outright before the message runs.
>
> `25000000000arune` is **1.25× the Cosmos-path floor of 20 × 10⁹ arune per gas**.
> At ~300,000 gas that is ~0.0075 RUNE. If you prefer a wider margin for a one-shot
> operation like this, `50000000000arune` (2×) also works.
>
> ⚠️ This value is tuned for **Cosmos CLI transactions** (`runed tx …`), which are
> checked against `min_gas_price`. **EVM transactions** (`eth_sendRawTransaction`, raw
> `runed tx evm raw`) are checked against the higher `min_regular_gas_price`
> (`2 500 000 000 000` arune/gas) and a `25 × 10⁹` price is **100× too low** there —
> see [tx-fees.md](./tx-fees.md). Wallets handle the EVM side; you only need this for
> `runed tx`.
>
> ⚠️ **The denom must be `arune`, not `rune`.** `--gas-prices` is parsed without
> denom normalization, so `rune`-denominated values never match the `arune`
> requirement and are rejected.
>
> The old `--fees 1000arune` (= 10⁻¹⁵ RUNE) was ~6 × 10¹² times too small.
>
> Run `runed query feemarket params --home ~/.runed` to read the current floor.

> ⚠️ **The `--amount` / `--pubkey` / `--moniker` flags do not exist.** In Cosmos SDK
> v0.50 those values are read from the JSON file only; passing them as flags is
> rejected. Keep the file — it documents the terms you bonded under.

### Step 7 — confirm you are in the active set

```bash
runed query staking validator $(runed keys show <key-name> --bech val -a --home ~/.runed) --home ~/.runed
runed query staking validators --home ~/.runed
```

Your validator is **bonded** once it is in the top **101** by voting power.

## Querying your validator

Every command below is read-only. `--home ~/.runed` and `--node tcp://localhost:26657`
are assumed; add them if your home or RPC address differ.

> Your validator's **JSON-RPC (8545) and REST API (1317) are disabled by default** —
> see [Node profiles](#node-profiles-what-runs-on-your-node). All queries here go
> through the **CometBFT RPC on 26657**, which stays bound to `127.0.0.1`.

### Identity

```bash
runed tendermint show-node-id     # node ID  — the <nodeID> in persistent_peers
runed tendermint show-address     # consensus address (runevalcons1…) — used by signing-info
runed tendermint show-validator   # consensus ed25519 pubkey
runed keys show <key-name> --bech val -a   # operator address (runevaloper1…)
```

### Liveness and jail risk — the most important checks

```bash
# Live status: height, catching_up, voting power.
runed status

# Signing info: the direct answer to "am I about to be jailed?"
runed query slashing signing-info $(runed tendermint show-address)
```

`signing-info` returns `index_offset` (blocks tracked in the current window) and
`missed_blocks_counter`. With a **10,000-block** window at 50% required, you are
jailed once `missed_blocks_counter` exceeds **5,000**. At 6s blocks the window is
~16.7h, so ~8.3h of downtime jails you.

```bash
runed query slashing signing-infos   # every validator at once
runed query slashing params          # window size, slash fractions, jail duration
```

> `Enumerable` missed-block bitmaps are **not** exposed by the gRPC query — only the
> counters above. `missed_blocks_counter` is the number that matters.

### Validator set

```bash
runed query staking validators                     # all validators, full detail
runed query staking validator <runevaloper…>       # just yours
runed query staking params                         # max validators, bond denom, unbonding time
runed query staking pool                           # bonded / not-bonded totals
runed query comet-validator-set                    # consensus set + voting power (CometBFT view)
runed query comet-validator-set 12345              # at a specific height
```

### Delegations and rewards

```bash
runed query staking delegations <rune1…>                    # your delegations
runed query staking delegations-to <runevaloper…>           # everyone delegating to you
runed query staking unbonding-delegations-from <runevaloper…>
runed query distribution rewards <rune1…>                   # claimable rewards
runed query distribution commission <runevaloper…>          # your commission
runed query distribution validator-outstanding-rewards <runevaloper…>
runed query distribution slashes <runevaloper…>             # slash history
```

### Balance and blocks

```bash
runed query bank balances <rune1…>
runed query bank balance <rune1…> arune
runed query block                 # latest block
runed query block 12345           # a specific height
runed query tx <HASH>
```

### Raw CometBFT RPC

Useful for monitoring scripts, since it returns JSON:

```bash
curl -s localhost:26657/status                | jq '.result.validator_info, .result.sync_info'
curl -s localhost:26657/validators            | jq '.result.validators'
curl -s localhost:26657/net_info              | jq '.result.n_peers'
curl -s localhost:26657/dump_consensus_state  | jq '.result.round_state.height'
```

`dump_consensus_state` is the quickest way to see a **stalled** chain: a height that
stops incrementing with `step` stuck at `RoundStepPropose` means validators are not
reaching agreement.

### Node profiles — what runs on your node

`runed setup` writes one of two profiles, chosen by your `v` (validator) or `s`
(seed) answer. **Validator is the default** and is deliberately locked down:

| Service | Port | Validator | Seed |
|---|---|---|---|
| p2p | 26656 | open | open |
| CometBFT RPC | 26657 | `127.0.0.1` only | `0.0.0.0` |
| REST API | 1317 | **disabled** | enabled |
| EVM JSON-RPC | 8545 / 8546 | **disabled** | enabled |
| telemetry | (scraped via 26657) | enabled | enabled |

A validator needs none of the disabled services: `create-validator` broadcasts over
**26657**, not REST, and dedicated public API / JSON-RPC nodes serve those endpoints.
Telemetry stays on because it opens no listening socket — it is scraped over the
loopback RPC path — and it is what lets you see uptime before a missed-block window
jails you.

To re-enable a service on a node you intend to publish, edit `~/.runed/config/app.toml`:

```toml
[api]
enable = true
address = "tcp://0.0.0.0:1317"

[json-rpc]
enable = true
address = "0.0.0.0:8545"
ws-address = "0.0.0.0:8546"
```

Only do this on a node you intend to expose — and firewall the ports accordingly.

## Staking parameters (Hoodium)

| Parameter | Value |
|-----------|-------|
| Max validators | 101 |
| Min self-delegation | 500,000 RUNE |
| Min commission rate | 5% |
| Max commission rate | 25% |
| **Unbonding time** | **14 days** |

> A minimum **delegation** of 0.1 RUNE is enforced at **both** delegation entry
> points: the staking precompile (`core/staking`, for EVM wallets) and the Cosmos
> ante chain (`app/ante/cosmos.MinDelegationDecorator`, for `runed tx staking
> delegate`). Cosmos staking has no min-delegation parameter, so Hoodium checks
> it itself — both paths read the same `types.MinDelegation` value, so neither can
> be used to bypass the floor.

### About the unbonding time

Hoodium sets **14 days**, applied explicitly in the genesis (rather than
inheriting the Cosmos default of 21 days).

**This single value covers both:**
- a **validator** unbonding (de-registering / losing bonded status), and
- a **delegator** undelegating their stake.

> ⚠️ Cosmos staking exposes **one** unbonding parameter. There is **no separate
> delegator timeframe** — the SDK applies the same period to delegations. So a
> delegator cannot have a shorter window than a validator without forking the
> staking module.

During the unbonding period the tokens are **locked and still slashable**; they
return to the account once the period elapses.

## Operating notes

- The bootstrap validator has **no special role** once others are bonded: if it
  goes down the chain keeps running, provided less than one third of voting power
  is offline.
- Only a validator's **public consensus key** is public; `config/priv_validator_key.json`
  never leaves your node. Back it up — losing it means you cannot sign.

## Unjailing after downtime

If your validator misses too many blocks it is **jailed** and stops earning. Once
the jail period has elapsed, unjail it.

**There is no `runed tx slashing unjail` command** — Hoodium exposes slashing
actions through the EVM, not as native Cosmos transactions. Call the **staking
precompile's `unjail` method** instead.

| | |
|---|---|
| Precompile address | `0x19be000000000000000000000000000000000003` |
| Method | `unjail(address validator)` |
| Argument | The validator's **operator address** (the `0x…` form of your `runevaloper1…`) |
| Caller | **Must be the operator itself** — the precompile rejects any other sender |

Because the caller must be the operator, call it **from the same EVM account that
owns the validator** (the account holding your self-delegation). An operator
address and its account address are the same 20 bytes, so the check is exact.

```javascript
// viem / ethers — send from the operator account
const abi = ["function unjail(address validator) returns (bool)"];
await walletClient.writeContract({
  address: "0x19be000000000000000000000000000000000003",
  abi,
  functionName: "unjail",
  args: ["0x<validator-operator-address>"],
});
```

> The method simply forwards to `x/slashing`'s unjail, so the usual preconditions
> apply: the jail period must have elapsed, and your self-delegation must still be
> at or above `min-self-delegation`. Calling it early fails with a clear error.

## Securing the node and keeping it running

A validator that stops is a validator that gets jailed. Two things keep it up:
a **systemd unit** so it starts on boot and restarts if it crashes, and sensible
host hardening so the node key is not exposed.

### 1. Run it as a dedicated user

Do not run a validator as `root`. Create a service account and give it the node
home only:

```bash
sudo useradd --system --home /home/runed --create-home --shell /usr/sbin/nologin runed
sudo chown -R runed:runed ~/.runed
sudo chmod 700 ~/.runed
```

Then move the node home under that account (or create it there from the start with
`runed setup --home /home/runed/.runed`).

> ⚠️ **`~/.runed/key_seed.json` holds your mnemonic in plaintext.** If that account
> is compromised, the premine and the validator's funds are gone. Keep the file mode
> `0600`, and keep the keyring directory (`~/.runed/keyring-file/`) at `0700`.

### 2. systemd service

Create `/etc/systemd/system/runed.service`:

```ini
[Unit]
Description=Hoodium node (runed)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=runed
Group=runed
# Raise if your node holds many open peers / RPC clients.
LimitNOFILE=65535
ExecStart=/usr/local/bin/runed start --home /home/runed/.runed
Restart=always
RestartSec=5
# Graceful shutdown so the last state is flushed before exit.
TimeoutStopSec=30
KillSignal=SIGINT
# Modest hardening; the node needs its own home writable.
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=read-only
ReadWritePaths=/home/runed/.runed
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Enable and start it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable runed      # start on boot
sudo systemctl start runed
systemctl status runed           # confirm it is running
sudo journalctl -u runed -f      # follow the logs
```

> **Adjust `ExecStart`** to the real binary path (`which runed`) and home directory.
> `ProtectHome=read-only` plus `ReadWritePaths` is what keeps the service account
> from writing anywhere except its node home.

### 3. Restart on reboot

`systemctl enable runed` already handles reboots — that is its purpose. If you
cannot use systemd, a **cron `@reboot`** entry is the fallback:

```bash
crontab -e -u runed
```

```cron
@reboot sleep 30 && /usr/local/bin/runed start --home /home/runed/.runed >> /home/runed/runed.log 2>&1
```

> Prefer systemd. `@reboot` gives you no restart-on-crash, no log integration and no
> graceful shutdown — it only covers the boot case.

### 4. Keep the validator signing

- **Back up `~/.runed/config/priv_validator_key.json`** and keep it offline. It is
  plaintext, and losing it means you can never sign again; leaking it means someone
  else can double-sign on your behalf.
- **Never copy a validator key to a second machine.** Running the same key twice
  produces an equivocation, which is slashed at **10%** and **tombstoned** — the
  validator is permanently removed from the set.
- **Monitor liveness.** Miss more than 50% of a 10,000-block window and you are
  jailed. `runed status --home ~/.runed` shows `catching_up` and the latest height.
- **Firewall the RPC ports.** `26657` (CometBFT RPC) and `8545` (EVM JSON-RPC) should
  not be exposed publicly unless you intend to serve them. Keep `26656` (p2p) open.
- **Do not enable the `personal` JSON-RPC namespace** — see
  [`evm-wallets.md`](./evm-wallets.md).
