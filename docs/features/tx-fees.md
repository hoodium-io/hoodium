# Transaction Fees — the Hoodium two-gas-price model

> **Unique to Hoodium.** Hoodium charges **two different minimum gas prices**: a
> lower one for *regular* transactions and a higher one for *contract
> deployments*. This deters deployment spam (chain-state bloat) while keeping
> ordinary transfers cheap.

## The two prices

| Transaction kind | Minimum gas price |
|---|---|
| **Regular** (non-deployment) | **0.0000025 RUNE** per gas |
| **Contract deployment** (empty `to` address) | **0.000005 RUNE** per gas |

The deployment minimum is **2×** the regular minimum.

> **Units:** gas prices flow through the chain in the base denomination,
> **arune** (1 RUNE = 10^18 arune). Internally these are `2.5e12` and `5e12`
> arune per gas respectively.

## How much is a transaction?

A transaction's fee is `gas used × gas price`. The **gas used** is charged by the
EVM (a protocol cost, not a Hoodium setting):

| Transaction | Typical gas | Cost |
|---|---|---|
| Simple transfer | 21,000 | **0.0525 RUNE** (regular rate) |
| ERC-20 transfer | ~65,000 | **0.1625 RUNE** (regular rate) |
| Contract deploy | ~150k–1.5M (size-dependent) | **0.75–7.5 RUNE** (deploy rate) |

> Contract-deployment gas is **not fixed**: `53,000` base + `~16 gas/byte` of
> bytecode + constructor execution. Larger contracts cost more automatically.

Note that **ERC-20 transfers and other contract calls** pay the *regular* rate —
the higher deployment rate is reserved for transactions that **create** a
contract.

## How a transaction is classified

A transaction is a **contract deployment** when its `to` address is **empty/nil**
(the standard EVM `CREATE` rule). Everything else — transfers *and* calls to
existing contracts — is a **regular** transaction.

> EIP-7702 (type `0x04`) authorization transactions carry a populated `to`, so
> they are classified as **regular**.

## Governance-adjustable

The two minimums are **`x/feemarket` module parameters**:

- `min_regular_gas_price`
- `min_deployment_gas_price`

They can be changed through **governance** (`MsgUpdateParams`) — so the fee
policy can be tuned (e.g. as the price of RUNE changes) **without a software
upgrade**.

## Where it is enforced

In the **ante handler**, as a fee floor. The decorator
`EthDeploymentGasPriceDecorator` (`app/ante/evm/deployment_fee.go`) runs right
after the global minimum-gas-price check and rejects any transaction whose **fee**
(`gas × gas price`) is below the minimum for its kind:

```
regular tx fee    < min_regular_gas_price    × gas  ->  rejected
deployment tx fee < min_deployment_gas_price × gas  ->  rejected
```

The check applies to both **CheckTx** and **DeliverTx** (skipped in simulation).
If both minimums are zero, the check is disabled.

## These are floors, not ceilings (EIP-1559 stays on)

Hoodium keeps the standard **EIP-1559 base fee**, which floats with block
congestion:

```
effective gas price  =  max(base fee + priority tip,  the applicable minimum)
```

- On a quiet chain, the base fee sits near the floor, so users effectively pay
  the minimum.
- Under congestion the base fee rises **above** the minimum (users pay more).
- The base fee **never** lets a user pay **below** the applicable minimum.

The base fee gives automatic congestion pricing and spam resistance; the two
minimums guarantee a floor.

## Why two prices?

- **Anti-spam:** contract deployment is the cheapest way to bloat chain state. A
  higher floor makes bulk-deployment spam more expensive (on top of the EVM's
  already-large deploy gas).
- **Cheap everyday use:** ordinary transfers and contract calls stay inexpensive.
- **Tunable:** the values are governance-adjustable as the economics evolve.

## Testing note

The two minimums are **disabled by default in the test suite** (set to zero in
`app/test_helpers.go` `init()`), so unrelated tests can use small gas prices. The
policy itself is covered by dedicated tests in
`app/ante/evm/deployment_fee_test.go`.