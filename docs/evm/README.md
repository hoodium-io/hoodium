# EVM

How Hoodium's EVM layer works and how to use it.

| Doc | What it covers |
|---|---|
| [Transaction mechanism](./evm-tx-mechanism.md) | Full lifecycle from signature to committed balance: ante chain, execution, gas refund, batch balance settlement, supply rules |
| [Compatibility](./evm-compatibility.md) | Fork/EIP support and the JSON-RPC method reference |
| [Wallets](./evm-wallets.md) | Connecting a wallet and sending a transaction |

## The one thing to understand first

Hoodium keeps **two** views of an account balance: `x/bank` (authoritative) and
the EVM's in-memory `stateObject` (a per-transaction cache). The EVM executes
against its own copy and hands the final balances back at commit, where they are
reconciled with the bank.

That reconciliation is the part that has bitten us in practice, so it has its own
section — see [`evm-tx-mechanism.md` §3](./evm-tx-mechanism.md#3-balance-settlement--the-core-mechanism).

## Related

- [`../precompile.md`](../precompile.md) — the precompile set (RUNE, HOODI, staking, price oracle)
- [`../tx-fees.md`](../tx-fees.md) — the two-gas-price model
- [`../technologies/pona/pona.md`](../technologies/pona/pona.md) — block rewards
