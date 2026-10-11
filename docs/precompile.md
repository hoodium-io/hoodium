# Precompiles

## Overview

Precompiles are built-in smart contracts that offer optimized operations within
the EVM. They provide specialized functionalities that extend beyond standard
Solidity-based contracts.

## Implementation

The precompile framework extends the CosmosSDK-based EVM implementation by
introducing built-in smart contracts executed natively by the chain. Hoodium's
precompiles provide token interfaces (RUNE, HOODI), price oracles, EVM access to
staking, and test helpers.

### Main Components

1. **Precompile Contracts:** The code for precompiles is located inside the
   [precompile](https://github.com/hoodium-io/runed/tree/main/precompile) directory.
   Each precompile defines its logic, supported methods, and how they
   interact with the EVM state.

2. **ABI and bytecode generation:** A precompile must provide an ABI located in
   `abi.json` and EVM bytecode located in `byte_code.go`. They both can be
   obtained by compiling a Solidity contract stored in Hardhat's
   [contract](https://github.com/hoodium-io/runed/tree/main/precompile/hardhat/contracts)
   directory.

3. **Versioning:** Precompiles can have multiple versions. The versions are
   handled by a version map.

4. **Method Execution:** Each precompile registers methods that follow the
   [Method](https://github.com/hoodium-io/runed/blob/main/precompile/method.go#L30)
   interface.

5. **Hardhat Integration:** Users can interact with precompiles using
   [Hardhat tasks](https://github.com/hoodium-io/runed/tree/main/precompile/hardhat/tasks),
   which provide CLI-based access for reading and writing data to the blockchain.

### Adding a New Precompile

Follow these steps to add a new precompile:

1. **Define the Solidity interface:** Create a Solidity interface. See example
   for [RUNE Token](https://github.com/hoodium-io/runed/tree/main/precompile/runetoken).

2. **Implement the Solidity caller contract:** Create a contract that calls the
   precompile. See example for
   [PriceOracleCaller](https://github.com/hoodium-io/runed/blob/main/precompile/hardhat/contracts/PriceOracleCaller.sol).

3. **Generate ABI and bytecode:** Use Hardhat to compile the contract and
   extract the `deployedBytecode` and `abi` from the artifacts.

4. **Implement the precompile logic:** Create a new package for the new
   precompile. Implement methods and register them. See example for
   [RUNE Token](https://github.com/hoodium-io/runed/tree/main/precompile/runetoken).

5. **Register the Precompile in the Chain:** Create an instance of the new precompile
   in the [Rune app](https://github.com/hoodium-io/runed/blob/main/app/app.go#L898)

6. **Write Hardhat Tasks:** Define new tasks in the
   [Hardhat tasks](https://github.com/hoodium-io/runed/tree/main/precompile/hardhat/tasks) directory.

## Available Precompiles

The following precompiles are registered in the Hoodium Mainchain. Addresses are
listed in their canonical order, which is **consensus-critical** — see
`DefaultPrecompilesVersions` in `x/evm/types/precompile.go`.

| Precompile | Address |
|---|---|
| RUNE Token | `0x19be000000000000000000000000000000000000` |
| HOODI Token | `0x19be000000000000000000000000000000000001` |
| Price Oracle | `0x19be000000000000000000000000000000000002` |
| Staking | `0x19be000000000000000000000000000000000003` |
| TestBed | `0x19BE100000000000000000000000000000000000` |

### RUNE Token Precompile

#### Overview

The RUNE Token precompile provides an interface for interacting with Hoodium's
native ERC-20 RUNE token — the chain's gas, governance and reward token. It
enables standard ERC-20 token operations such as balance queries, transfers and
approvals.

#### Address

```
0x19be000000000000000000000000000000000000
```

#### Methods

<!-- markdown-link-check-disable-next-line -->
See [IRUNE](https://github.com/hoodium-io/runed/blob/main/precompile/runetoken/IRUNE.sol)
interface for a list of methods.

### HOODI Token Precompile

#### Overview

The HOODI Token precompile provides an interface for interacting with the HOODI
ERC-20 token (denom `ahoodi`), reserved for the Yieldchain. It enables standard
ERC-20 token operations such as balance queries, transfers and approvals.

#### Address

```
0x19be000000000000000000000000000000000001
```

### Price Oracle Precompile

#### Overview

The Price Oracle precompile provides access to price data for assets tracked
by the Hoodium blockchain. It enables querying the latest price updates and
retrieving asset price information with a predefined level of precision.

#### Address

```
0x19be000000000000000000000000000000000002
```

#### Methods

See [IPriceOracle](https://github.com/hoodium-io/runed/blob/main/precompile/priceoracle/IPriceOracle.sol)
interface for a list of methods.

### Staking Precompile

#### Overview

The Staking precompile exposes Cosmos staking operations to the EVM, letting
EVM accounts create validators, delegate, undelegate and query staking state
without leaving the EVM toolchain.

#### Address

```
0x19be000000000000000000000000000000000003
```

### TestBed Precompile

#### Overview

The TestBed precompile provides Hoodium-specific test helpers used for
development and integration testing.

#### Address

```
0x19BE100000000000000000000000000000000000
```
