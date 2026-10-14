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
| **PoNA (RUNE rewards) — `x/pona`** | [`pona.md`](./pona.md) | Implemented |
| RUNE emission schedule (50/25/10) | [`pona.md`](./pona.md#0-the-module-at-a-glance) | Implemented |
| Two-gas-price model | [`../features/tx-fees.md`](../features/tx-fees.md) | Implemented |

> Add a new `<technology>.md` here and link it in the table above when a Hoodium
> mechanism is introduced.
