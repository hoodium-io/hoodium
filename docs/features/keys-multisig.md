# Keys Multisig — K-of-N threshold accounts

> **Distinct from the Solidity multisig.** This document covers the **plain
> Cosmos multisig key** — a *cryptographic key type* built into `runed keys`,
> which requires no smart contract and no extra module. For the on-chain,
> contract-based multisig intended for public/community use, see the separate
> Solidity multisig work (roadmap item B16).

## What it is

A multisig account is a **key**, not a contract and not a module. Its address is
derived from a list of member public keys plus a threshold `K` of `N`. To spend
from it, `K` of the `N` members must sign the same transaction.

Under the hood this is `crypto/keys/multisig.LegacyAminoPubKey` — a CometBFT/
Cosmos key type that already ships with the SDK, so `runed` supports it today
with no configuration.

| | Plain multisig key (this doc) | Solidity multisig |
|---|---|---|
| Where it lives | Node keyring | EVM contract |
| Requires a module | **No** | No (contract), but needs a deployment |
| Spend path | Cosmos tx (`runed tx ...`) | EVM call |
| Audience | Operators, teams, treasury | Public/community users |
| Status | **Available now** | Not started (roadmap B16) |

## How the address is derived

The multisig address is `AddressHash(multisigPubKey.Bytes())` — a hash of the
**threshold plus the member public keys**. Consequences worth knowing:

- The address is **deterministic**. The same member keys and threshold always
  produce the same address, on any machine.
- The address **depends on member order** unless you sort. `runed` sorts members
  by address by default, so the common case is reproducible; pass `--nosort` only
  if you need a specific supplied order.
- **Changing members or the threshold changes the address.** There is no "add a
  member" operation — you build a *new* multisig and migrate funds.

> ⚠️ A multisig address holds funds like any other account, but it **cannot**
> sign with a single private key. Treat the member keys as the real asset: lose
> `N - K + 1` of them and the funds are unrecoverable.

## Prerequisites

Each member needs their own key in the **same keyring** before the multisig can
be assembled, because `--multisig` takes key *names* from the local keyring.

```bash
# On each member's machine, create a personal key (or import an existing one)
runed keys add alice --keyring-backend file --home ~/.runed
runed keys add bob   --keyring-backend file --home ~/.runed
runed keys add carol --keyring-backend file --home ~/.runed
```

Then collect the **public keys** and distribute them to whoever will assemble the
multisig (typically the treasury operator):

```bash
# Each member exports their public key (safe to share)
runed keys show alice --pubkey --keyring-backend file --home ~/.runed
```

> 🔑 Only **public** keys are shared. Never move a member's private key or

## Creating a 2-of-3 multisig

Once the member keys exist in one keyring:

```bash
runed keys add foundation \
  --multisig alice,bob,carol \
  --multisig-threshold 2 \
  --keyring-backend file \
  --home ~/.runed
```

Flags:

| Flag | Meaning |
|---|---|
| `--multisig` | Comma-separated list of member key **names** in the keyring |
| `--multisig-threshold` | `K` — how many signatures are required |
| `--nosort` | Keep the supplied member order instead of sorting by address |

Verify the result:

```bash
# Show the multisig account address
runed keys show foundation -a --keyring-backend file --home ~/.runed

# Inspect the record (shows threshold + members)
runed keys show foundation --keyring-backend file --home ~/.runed

# Confirm it is listed alongside the member keys
runed keys list --keyring-backend file --home ~/.runed
```

Fund it like any account:

```bash
runed tx bank send <your-key> <multisig-address> 1000000arune \
  --chain-id rune_6590-1 --keyring-backend file --home ~/.runed
```

## Signing a transaction

A multisig spend is **not** a single command. The flow is: build once, sign by
`K` members, then broadcast.

```bash
# 1. Generate the unsigned transaction
runed tx bank send <multisig-address> <recipient> 500arune \
  --from foundation \
  --chain-id rune_6590-1 \
  --keyring-backend file --home ~/.runed \
  --generate-only > unsigned.json

# 2. Each required member signs the SAME unsigned.json
runed tx sign unsigned.json \
  --from alice \
  --multisig <multisig-address> \
  --chain-id rune_6590-1 \
  --keyring-backend file --home ~/.runed \
  --output-document alice-sig.json
# ... repeat with bob / carol until K signatures exist

# 3. Combine the signatures into one multisig signature
runed tx multisign unsigned.json foundation alice-sig.json bob-sig.json \
  --chain-id rune_6590-1 \
  --keyring-backend file --home ~/.runed \
  --output-document signed.json

# 4. Broadcast
runed tx broadcast signed.json \
  --chain-id rune_6590-1 \
  --keyring-backend file --home ~/.runed
```

> ⚠️ **All members must sign the identical `unsigned.json`.** If the transaction
> body, fee, sequence, or account number differ between signers, the combined
> signature is invalid. Pass the same file around — do not rebuild it per member.

## Operational notes

- **Threshold choice.** `2-of-3` tolerates one lost key while still requiring two
  approvals. `K = N` is safest but brittle — one lost key freezes the account.
- **Sequence numbers.** Each member signs with the *multisig account's* sequence,
  not their own. Signatures gathered against a stale sequence will fail to
  broadcast.
- **Ledger support is not available.** `runed`'s Ledger derivation path panics
  with `"not supported yet"` (`crypto/keyring/options.go`), so multisig members
  must use file/OS keyring keys.
- **No on-chain registry.** The multisig exists only as key material. Back up
  every member's mnemonic, and record the member list plus threshold somewhere
  durable — the account can be rebuilt from public keys alone.

## Recovering a multisig

Because the address is derived from public keys, you can rebuild the record on a
fresh machine once each member's key is present:

```bash
runed keys add foundation \
  --multisig alice,bob,carol \
  --multisig-threshold 2 \
  --keyring-backend file --home ~/.runed
```

The address will match the original **provided the member order and threshold are
the same**. This is why the default sorting matters, and why `--nosort` should
only be used deliberately.

## Related

- [`tx-fees.md`](./tx-fees.md) — the fee model applied to any multisig spend
- [`../validator.md`](../validator.md) — validator operator keys
- Solidity multisig (public/community use) — roadmap item **B16**

> mnemonic between machines.
