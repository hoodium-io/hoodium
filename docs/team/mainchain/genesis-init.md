# Mainchain Genesis Initialization (TEAM ONLY)

> **⚠️ TEAM USE ONLY.** This document is for the **Hoodium team** and describes the
> command used to generate the **official Hoodium Mainchain genesis** at network
> launch (devnet, testnet or mainnet).
>
> **If you are a validator or node operator who wants to join an already-running
> network, this is NOT the document for you** — see
> [Node Setup](../../node-setup.md) and [Validator Setup](../../validator.md).

## The command

**Single node (the default — production case):**

```
runed genesis init-files --chain-id rune_6590-1
```

This writes the genesis and a ready-to-run node home **directly into the node home
directory** (default: `~/.runed`). There is **no wrapper folder and no `node0/`**
step — you can run `runed start` immediately afterwards.

**Multi-node (local network only):**

```
runed genesis init-files --val 4 --home ./.localnet --starting-ip-address 192.168.10.2
```

Only `--val N` with `N > 1` switches to the `<home>/node0/runed/` layout, for
spinning up several nodes side by side on one host (Docker / localnet). **This is
not how a public network is launched.**

### What each flag does

| Flag | Default | Needed? | Purpose |
|---|---|---|---|
| `--chain-id` | *random* | **Yes, effectively** | Selects the network (`rune_6590-1` / `rune_6591-1` / `rune_6592-1`). If omitted you get a **random** chain ID, which is only useful for throwaway localnets. |
| `--home` | the node home (`~/.runed`) | Optional | The node home directory. In single-node mode this **is** the node home — same flag name and meaning as every other Cosmos command. |
| `--val` | `1` | Optional | `1` = a single production node home (default). `N > 1` = N node homes in a local-network layout. |
| `--starting-ip-address` | `localhost` | Only for `--val > 1` | Pre-wires each node's P2P/RPC/API/JSON-RPC addresses for a multi-node local network. |
| `--node-dir-prefix` | `node` | Only for `--val > 1` | Folder prefix for each node in the multi-node layout. |
| `--node-daemon-home` | `runed` | Only for `--val > 1` | Inner home folder name in the multi-node layout. |

### Where files go

**Single node (default)** — straight into the node home:

```
~/.runed/
  config/
    genesis.json
    config.toml
    app.toml
    client.toml
    priv_validator_key.json
    node_key.json
  data/
```

**Multi node (`--val N`, N > 1)** — one folder per node:

```
./.localnet/
  node0/runed/   ← validator 0's home (config/, data/)
  node1/runed/   ← validator 1's home
  node2/runed/   ← ...
```

> ⚠️ **Cleanup safety.** On failure the generator removes the output directory —
> but **only in multi-node mode**, where the directory is something the command
> just created. In single-node mode (the default) the output directory is a real
> node home that may already hold a live node, so it is **never deleted**.

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

## What lands in each node's `config.toml`

This is important to understand before distributing nodes across real servers,
because **the generated IP addresses are baked into every node's config**.

At generation the command writes, for **each** node `i`:

```go
nodeConfig.P2P.PersistentPeers = persistentPeers   // ALL nodes, pre-wired
nodeConfig.RPC.ListenAddress   = getRPCAddress(i, args.startingIPAddress)
nodeConfig.P2P.ListenAddress   = getP2PAddress(i, args.startingIPAddress)
nodeConfig.RPC.PprofListenAddress = getPprofAddress(i, args.startingIPAddress)
nodeConfig.P2P.AddrBookStrict  = getAddrBookStrict(args.startingIPAddress)
nodeConfig.P2P.AllowDuplicateIP = getAllowDuplicateIP(args.startingIPAddress)

appConfig.API.Address        = getAPIAddress(i, args.startingIPAddress)
appConfig.GRPC.Address       = getGRPCAddress(i, args.startingIPAddress)
appConfig.JSONRPC.Address    = getJSONRPCAddress(i, args.startingIPAddress)
appConfig.JSONRPC.WsAddress  = getJSONRPCWsAddress(i, args.startingIPAddress)
```

So **yes — there is a built-in node-IP helper**, and it works like this:

- Each node's `config.toml` gets a **`persistent_peers` list containing *every*
  generated node**, built from each validator's memo
  (`nodeID@<ip>:26656`, comma-joined and sorted).
- Each node's listen addresses are offset by index `i`, so nodes do not collide
  on one host.
- With `--starting-ip-address localhost`, the peers become
  `nodeID@localhost:<26656 + 2*i>` — the "many nodes on one machine" variant.
- With a real IP base, node `i` is assigned `<base+i>`.

**What this means in practice:** the generated configs are a complete, consistent,
mutually-wired set — correct as-is **only if** the nodes genuinely keep those IPs.

| Scenario | What to do |
|---|---|
| All nodes run at the generated IPs (Docker, same subnet) | **Use as generated.** Nothing to edit. |
| Nodes deployed to real/different servers | The baked IPs will be wrong. Either pass the correct base IP at generation time, **or** edit each node's `config.toml` `persistent_peers` / `listen-address` after generating. |

> ⚠️ **No IP summary is printed.** The command's only output is
> `Successfully initialized N node directories`. It does **not** print a
> node-ID → IP table. To build one, read each generated node's
> `config/config.toml` (`persistent_peers` already contains every node's
> `nodeID@ip:26656`), or run `runed tendermint show-node-id --home <nodeHome>`
> per node.

## Full bootstrap-validator walkthrough

> **This is the only flow a bootstrap validator needs.** `init-files` does
> everything — it is **not** a first step that you then follow with `init`,
> `keys add` and `gentx`. Those are the *joining* flow for a different command
> (`runed init`), and are explained at the end of this section so you can see
> why they are **not** part of this flow.

### Why no moniker on the command line?

You **do** get a moniker — it is **derived from the home directory name** rather
than typed as a flag. With `--home ~/.runed` the moniker becomes `runed`; with
`--home ./mynode` it becomes `mynode`.

The reason there is no `--moniker` flag is that a single-node generation writes
**one** home, so there is exactly one name to derive, and deriving it keeps the
command short. (In multi-node mode each node is named `node0`, `node1`, … — see
`nodeDirName()` in `cmd/runed/chain.go`.)

If you want a specific moniker, just name the home directory:

```
runed genesis init-files --chain-id rune_6590-1 --home ~/.runed-hoodium-mainnet
# → moniker "runed-hoodium-mainnet"
```

### Step 1 — generate the node home

```bash
runed genesis init-files --chain-id rune_6590-1
```

This one command does **all** of the following:

| What | Result |
|---|---|
| Creates the node home | `~/.runed/` (config/, data/) |
| Node identity keys | `config/priv_validator_key.json`, `config/node_key.json` |
| Node config | `config/config.toml`, `config/app.toml`, `config/client.toml` |
| **Wallet key** | created in the keyring, **mnemonic saved to `config/key_seed.json`** |
| **Premine** | 9B RUNE + 10M HOODI → that wallet; 1B RUNE → `validator_reward_pool` |
| **The validator** | written **directly into the staking genesis** (self-delegated 500,000 RUNE) |
| Consensus params | `MaxGas = 10,000,000`, vote extensions enabled from block 1 |
| `config/genesis.json` | complete and ready to run |

> 🔑 **The wallet key is created for you.** Its mnemonic is written to
> **`~/.runed/config/key_seed.json`** — back this file up immediately and keep it
> secret; it controls the premine.
>
> 💡 **There is NO gentx step.** Because this is the chain's *birth*, the
> generated validator is written straight into the staking genesis
> (`staking.validators`, `delegations`, `last_validator_powers`). There is no
> `gentxs/` directory and nothing to `collect-gentxs`. Gentx is only used when
> *joining an existing* chain via `runed init`.

### Step 2 — start the chain

```bash
runed start --home ~/.runed
```

That is it — the chain is producing blocks from a single validator.

### Step 3 (optional) — confirm it is running

```bash
runed status --home ~/.runed
runed query staking validators --home ~/.runed
runed query bank balances <your-rune1-address> --home ~/.runed
```

Get your address with `runed keys list --home ~/.runed`, and see
[Accessing the premine from an EVM wallet](#accessing-the-premine-from-an-evm-wallet).

### ⛔ Why `runed init` / `keys add` / `gentx` are NOT part of this flow

You will see this sequence in other Cosmos docs (and it was, incorrectly, in an
earlier version of this page):

```bash
runed init <moniker> --chain-id <chain-id> --home <home>     # ← do NOT run after init-files
runed keys add <key-name> --keyring-backend file --home <home>
runed genesis gentx <key-name> <amount> --pubkey "$(runed tendermint show-validator)" ...
```

**Those three commands are the wrong tool here, for three reasons:**

1. **`runed init` only writes a *default* genesis.** It has **no premine logic** —
   no 9B RUNE, no `validator_reward_pool`, no bonded-pool funding. Running it
   after `init-files` would **overwrite your real genesis** with an empty one.
   (`runed init` refuses if `genesis.json` exists unless you pass
   `--overwrite` — do not pass it.)
2. **`runed keys add` would create a *second* wallet.** `init-files` already
   created one and funded it with the premine. A fresh key would hold nothing.
3. **`gentx` is unnecessary** (see the note above): the validator is already in
   the staking genesis.

They exist for **joining an existing network**, where there is no premine to
allocate and no genesis to author — the joiner only contributes a gentx (or a
`create-validator` tx). That flow is documented in
[Node Setup](../../node-setup.md) and [Validator Setup](../../validator.md).

### Quick reference

| Need | Command |
|---|---|
| Launch a brand-new chain (team) | `runed genesis init-files --chain-id <id>` → `runed start` |
| Join an existing chain (validator) | `runed setup` (see [Validator Setup](../../validator.md)) |

## How the module-account pools work

`validator_reward_pool` and `bonded_tokens_pool` are **Cosmos module accounts**,
not contracts or EOAs. Understanding this answers most of the usual confusion.

### Are they smart contracts? Do they need deploying?

**No — and no.** They are not Solidity contracts and there is **nothing to
deploy**. They are ordinary Cosmos *accounts*, created and owned by the chain
itself, whose addresses are **pure functions of their name**:

```
address = ripemd160(sha256(moduleName))
```

- `validator_reward_pool` → `0x4967715BC2Ba5CC2A262054A5a893e2E90FCD3f9`
- `bonded_tokens_pool`     → `0xD99070859237f882b18EAabFDD0849a386aB42cF`

Because the input is a **compile-time constant**, the address is **fixed for all
time** — it is the same on devnet, testnet and mainnet, and it was never
"generated" per launch. What varies per install is only the **premine owner's**
address (your own `keys add` account), which *is* generated at genesis time.

### Do they have private keys?

**No.** A module account has **no private key and no mnemonic** — nobody can sign
for it. It is not a wallet. Instead, **modules move funds out of it** through the
bank keeper, and `maccPerms` in `app/app.go` decides which module is allowed to
do what:

```go
maccPerms = map[string][]string{
    stakingtypes.BondedPoolName:              {authtypes.Burner, authtypes.Staking},
    stakingtypes.NotBondedPoolName:           {authtypes.Burner, authtypes.Staking},
    runerewardstypes.ValidatorRewardPoolName: nil,          // no extra perms
    runerewardstypes.ModuleName:              {authtypes.Minter},
    ...
}
```

So `validator_reward_pool` is money that **only `x/runerewards` can spend**, via
`DistributeRuneBlockReward` — and it needs no key to do so. That is the security
property: the reward reserve is unspendable by any human.

### When are they created?

**Automatically at chain start (`InitGenesis`), by the auth module** — not by this
command, and not manually. The SDK creates a module account lazily the first time
it is addressed:

```go
// cosmos-sdk/x/auth/keeper/keeper.go
acc := ak.GetAccount(ctx, addr)
if acc != nil { return ... }              // exists already
macc := types.NewEmptyModuleAccount(moduleName, perms...)  // else create it
```

### What this command actually does for them

It only **writes a bank balance into genesis** for the pool address:

```go
rewardPoolAddr := authtypes.NewModuleAddress(runerewardstypes.ValidatorRewardPoolName)
banktypes.Balance{
    Address: rewardPoolAddr.String(),
    Coins:   sdk.NewCoins(sdk.NewCoin(cmdcfg.BaseDenom, runeReserveAmount)),
}
```

At `InitGenesis` the bank module credits that balance, and the auth module creates
the matching module account. From the first block onward, `x/runerewards` pays
block rewards out of it. The same pattern funds the bonded pool, which is why the
staking module does not panic at `InitChain` with
*"bonded pool balance is different from bonded coins"*.

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
