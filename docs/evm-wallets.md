# EVM wallets — connecting and sending

Hoodium exposes a standard **Ethereum JSON-RPC** interface, so any EVM-compatible
wallet or tool can connect and transact. No wallet-specific integration exists or
is needed — the node speaks the plain geth JSON-RPC API.

## Connection parameters

| Setting | Value |
|---|---|
| RPC URL | `http://<node-host>:8545` (JSON-RPC), `ws://<node-host>:8546` (WebSocket) |
| Chain ID | `6590` (mainnet) · `6591` (testnet) · `6592` (devnet) |
| Currency symbol | `RUNE` |
| Decimals | `18` |
| Block explorer | — (none yet) |

The chain ID is returned by `eth_chainId` and matches the Cosmos chain-id prefix
(`rune_6590`, `rune_6591`, `rune_6592`).

> The node's JSON-RPC listener is configured in `app.toml` under `[json-rpc]`
> (`address`, `ws-address`, `api`). By default it binds `0.0.0.0:8545`, so expose
> it behind a reverse proxy or firewall if the node is public.

## Supported wallets and tools

Because the interface is standard Ethereum, all of these work unchanged:

| Category | Examples |
|---|---|
| Browser wallets | MetaMask, Rabby, Frame, Coinbase Wallet, Trust Wallet, OKX |
| Hardware | Ledger, Trezor (via their EVM integrations) |
| Desktop | Frame, Rabby Desktop |
| Programmatic | `ethers.js`, `viem`, `web3.js`, `web3.py`, Foundry `cast`, Hardhat |

Adding a network is the same in each: point it at the RPC URL and supply chain ID
`6590`/`6591`/`6592` with 18 decimals.

## Sending a transaction

Any wallet performs the same two steps:

1. **Build and sign locally** with the account's private key.
2. **Broadcast** the signed payload via `eth_sendRawTransaction`.

The node never sees the private key — it verifies and includes the signed
transaction. Import the account (private key or seed phrase) into the wallet, and
RUNE transfers work as ordinary value transfers.

RUNE is the EVM denomination (`evm_denom = "arune"`), so the native balance is
directly spendable, and ERC-20 tokens deployed on Hoodium behave normally.

### Gas

The chain runs EIP-1559 with a fee market. Wallets estimate gas automatically via
`eth_estimateGas` and read the current base fee via `eth_gasPrice` /
`eth_feeHistory`. A minimum gas price floor applies (see
[`features/tx-fees.md`](./features/tx-fees.md) for the two-gas-price model), so very low fee
settings will be rejected.

### Replay protection

Only EIP-155 replay-protected transactions are accepted by default
(`allow_unprotected_txs = false`). Every mainstream wallet signs EIP-155, so this
is transparent in practice.

## ⚠️ Do not enable the `personal` namespace

`personal` (node-side key management and signing) is available but **deliberately
excluded** from the default JSON-RPC namespaces. `app.toml` ships:

```toml
[json-rpc]
api = "eth,net,web3,rune"
```

Enabling `personal` would expose **unauthenticated node-side signing**:

- `personal_sendTransaction` signs with a key held in the **node's keyring**, with
  no password prompt.
- `personal_importRawKey` can plant arbitrary keys on the node.
- `personal_unlockAccount` / `personal_lockAccount` are **not supported** (they
  return `false`), so there is no unlock step standing between a caller and the
  node's keys.

On a validator this is a direct path to its funds. **Use `eth_sendRawTransaction`
instead** — every EVM wallet signs locally, which is both safer and simpler.

The same applies to the other available-but-off namespaces: `debug`, `txpool`, and
`miner`.

## Related

- [`features/tx-fees.md`](./features/tx-fees.md) — the two-gas-price model and fee floors
- [`evm-compatibility.md`](./evm-compatibility.md) — full JSON-RPC method reference
- [`features/keys-multisig.md`](./features/keys-multisig.md) — Cosmos-side K-of-N multisig accounts
