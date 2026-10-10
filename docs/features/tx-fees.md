# Transaction Fees — the Hoodium two-gas-price model

> **Unique to Hoodium.** Hoodium charges **two different minimum gas prices**:
> a low one for *regular* transactions and a much higher one for *contract
> deployments*. This deters deployment spam while keeping ordinary transfers
> cheap.

## The two prices

| Transaction kind | Minimum gas price |
|---|---|
| **Regular** (transfer, contract call, etc.) | **0.0001 RUNE** per gas |
| **Contract deployment** (empty `to` address) | **0.01 RUNE** per gas |

The deployment minimum is **100×** the regular minimum.

> **Units:** gas prices flow through the chain in the base denomination,
> **arune** (1 RUNE = 10^18 arune). Internally these are `1e14` and `1e16`
> arune per gas respectively.

Fees are computed as `gas used × gas price`. Illustrative costs:

| Transaction | Gas | At regular price | At deployment price |
|---|---|---|---|
| Simple transfer | 21,000 | 2.1 RUNE | — |
| ERC-20 transfer | 65,000 | 6.5 RUNE | — |
| Contract deploy | 1,500,000 | — | 15,000 RUNE |

## How a transaction is classified

A transaction is treated as a **contract deployment** when its `to` address is
**empty/nil** — the standard EVM rule for a `CREATE`. Everything else (including
a call to an existing contract) is a **regular** transaction.

> Note: EIP-7702 (type `0x04`) authorization transactions carry a populated `to`
> and are therefore classified as **regular**, not deployments.

## Where it is enforced

In the **ante handler**, as a fee floor. The decorator
`EthDeploymentGasPriceDecorator` (`app/ante/evm/deployment_fee.go`) runs right
after the global minimum-gas-price check and rejects any transaction whose
**effective gas price** is below the minimum for its kind:

```
regular tx gas price  < 0.0001 RUNE  -> rejected
deployment gas price  < 0.01   RUNE  -> rejected
```

The check applies to both **CheckTx** and **DeliverTx** (and is skipped in
simulation).

## These are floors, not ceilings

The two values are **minimums**. Hoodium keeps the standard **EIP-1559 base
fee**, which floats with block congestion. In practice:

```
effective gas price  =  max(base fee + priority tip,  the applicable minimum)
```

- On a quiet chain, the base fee sits at/near the floor, so users effectively pay
  the minimum.
- Under congestion, the base fee can rise **above** the minimum (users pay more).
- The base fee **never** lets a user pay **below** the applicable minimum.

## Values are genesis constants

Both prices are **compile-time constants** in
`app/ante/evm/deployment_fee_config.go`:

```go
RegularGasPriceMin    = 1e14 arune / gas  // 0.0001 RUNE / gas
DeploymentGasPriceMin = 1e16 arune / gas  // 0.01   RUNE / gas
```

They are **not** governance-adjustable parameters. Changing them requires a
**software upgrade**. `ValidateTwoGasPriceConfig()` fails fast at startup if the
constants are misconfigured (e.g. deployment price below the regular price).

## Why two prices?

- **Anti-spam:** contract deployment is the cheapest way to bloat chain state.
  A 100× higher floor makes bulk-deployment spam economically unattractive.
- **Cheap everyday use:** ordinary transfers and contract calls stay inexpensive.
- **Predictable:** fixed constants mean fee behaviour does not drift with
  governance.

## Enabling / tuning

To change the values, edit `app/ante/evm/deployment_fee_config.go` and ship a new
binary. There is no runtime flag.