# Hoodium Network Features

This directory documents features that are **unique to the Hoodium network** —
things Hoodium does differently from a stock Cosmos/EVM chain.

| Feature | Doc |
|---|---|
| Two-gas-price model | [`tx-fees.md`](./tx-fees.md) |
| Slashing & validator accountability | [`slashing.md`](./slashing.md) |
| Multisig keys (K-of-N threshold accounts) | [`keys-multisig.md`](./keys-multisig.md) |
| EVM wallets (connect & send) | [`evm-wallets.md`](./evm-wallets.md) |
| Guided first-run setup (`runed setup`) | [`../node-setup.md`](../node-setup.md) |
| PoNA (RUNE rewards) | [`../technologies/pona.md`](../technologies/pona.md) |
| `x/pona` (Proof of Network Activity) | [`../technologies/pona.md`](../technologies/pona.md) |

> Add a new `<feature>.md` here and link it in the table above when a Hoodium
> feature is introduced.
>
> Mechanism-level documentation (how a module works internally) lives in
> [`../technologies/`](../technologies/README.md) instead.
