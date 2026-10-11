# Networks

Hoodium defines **three distinct network identities**, each with its own
EIP-155 chain ID. They run the **same client binary** and the **same production
configuration** — only the chain ID (and therefore the genesis) differs.

| Network | Chain ID | Purpose |
|---|---|---|
| **Mainnet** | `rune_6590` | Production network. |
| **Testnet** | `rune_6591` | Stable public testing network. |
| **Devnet** | `rune_6592` | Temporary development network used to validate the current build before promoting it to testnet and then mainnet. **A full mainnet replica**, not a localnet. |

The chain ID constants live in [`utils/utils.go`](../utils/utils.go)
(`MainnetChainID`, `TestnetChainID`, `DevnetChainID`), with `IsMainnet`,
`IsTestnet` and `IsDevnet` helpers.

## Devnet

The devnet is the network the community can watch during active development. It
carries a distinct chain ID so it can never share an identity with testnet or
mainnet, while running the same production configuration as mainnet.

A public devnet is **not yet live** — see the roadmap in the
[README](../README.md).

## Node ports

A Hoodium node exposes the following services:

| Service       | Port  |
|---------------|-------|
| P2P           | 26656 |
| RPC           | 26657 |
| REST          | 1317  |
| GRPC          | 9090  |
| JSON-RPC HTTP | 8545  |
| JSON-RPC WS   | 8546  |

## Joining a network

To join an existing network as a validator or a seed node, use the guided
first-run setup — see [validator setup](./validator-setup.md).

```bash
runed setup
```

The guided setup lets you pick the network (`mainnet` | `testnet` | `devnet`),
the node type (`validator` | `seed`), and then walks you through initialising the
node, creating your wallet key and (for validators) creating the genesis
transaction.

## Genesis artifacts

Per-network genesis artifacts (genesis file, seeds, persistent peers) are
distributed by the Hoodium team for each network once it is live. Adding another
network means adding its chain ID to `utils/utils.go` and producing its genesis;
no new subcommand is required.