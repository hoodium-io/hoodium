# Hoodium Network Technologies

This directory documents the **technologies and protocol mechanisms** that make up
the Hoodium network — the custom machinery layered on top of the Cosmos SDK and
EVM stack.

Distinction from [`../features/`](../features/README.md):

| Directory | Talks about |
|---|---|
| `../features/` | User-facing **features** — what a holder, validator, or operator can do |
| `./` (this dir) | The **mechanisms** behind those features — the modules, algorithms, and economics |

| Technology | Doc | Status |
|---|---|---|
| `x/runerewards` block rewards | [`runerewards.md`](./runerewards.md) | Implemented (Tier-3 rate correction pending) |
| `x/pona` (Proof of Network Activity) | [`pona.md`](./pona.md) | Designed |
| RUNE emission schedule (50/25/10) | [`runerewards.md`](./runerewards.md#4-emission-schedule) | Designed |
| Two-gas-price model | [`../features/tx-fees.md`](../features/tx-fees.md) | Implemented |

> Add a new `<technology>.md` here and link it in the table above when a Hoodium
> mechanism is introduced.
