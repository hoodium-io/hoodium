# Precompile Hardhat Tasks

*Node.js v20 or greater is recommended*

Before getting started make sure you have changed to the hardhat directory and have installed the required
dependencies:

```
cd precompile/hardhat
npm install
```

## Networks

Hardhat is configured with two supported networks:

* `localhost` for connecting to local dev net (localnet-docker, localnet-bin).
* `testnet` for connecting to the public testnet.

All tasks and scripts support hardhat's global options which includes the `--network` flag. `localhost` is
used by default if no network is set. If running a task or script against `testnet` ensure you include
`--network testnet` as a hardhat argument.

## Accounts

Hardhat [vars](https://hardhat.org/hardhat-runner/docs/guides/configuration-variables) are used for configuring
accounts/keys.

```
WARNING

Configuration variables are stored in plain text on your disk. Avoid using this feature for data you wouldn’t
normally save in an unencrypted file. Run npx hardhat vars path to find the storage's file location.
```

You can determine what `vars` are available by running:

```
npx hardhat vars setup
```

The vars can be set to either a) a single private key, or b) a comma separated list (no whitespace) of private keys.

```
npx hardhat vars set RUNE_ACCOUNTS
```

And can be removed with:

```
npx hardhat vars delete RUNE_ACCOUNTS
```

## Scripts

Some helper scripts are included for convenience. It is recommended to run these scripts using `hardhat run` as they
may not execute as expected if running directly with node.

### accounts

Lists available accounts.

```
npx hardhat --network localhost run scripts/accounts.ts
```

### localhost-keys

Reads seed phrases from `build` dir used by localhost based localnets and prints a comma separated list of private
keys.

```
npx hardhat run scripts/localhost-keys.ts
```

This output can be used to easily set `RUNE_ACCOUNTS` for `localhost` use.

```
npx hardhat run scripts/localhost-keys.ts | npx hardhat vars set RUNE_ACCOUNTS
```

## Tasks

We name Hardhat [Tasks](https://hardhat.org/hardhat-runner/docs/advanced/create-task) with a precompile prefix. This
provides clarity on which precompile the task runs against, and keeps the output of `npx hardhat help` clean as
precompile tasks get grouped together. You can view the available tasks with:

```
npx hardhat help
```

*Note: The default Hardhat tasks are still visible (e.g `compile`), many of these will do nothing as we are only using
Hardhat for tasks/testing.*

Help information for a specific task can be obtained using

```
npx hardhat help <TASK>
```

e.g:

```
npx hardhat help priceOracle:latestRoundData
```

```
Usage: hardhat [GLOBAL OPTIONS] priceOracle:latestRoundData

OPTIONS:

  (none)

priceOracle:latestRoundData: Returns the price from the last update tick (round) of the oracle
```

Here we can see the priceOracle:latestRoundData task takes no arguments - correct usage would be:

```
npx hardhat --network testnet priceOracle:latestRoundData
```

## Generating precompile ABI + EvmByteCode

Custom precompiles are Go-native, but external services detect them as smart contracts by calling
`eth_getCode`. To satisfy that, each precompile embeds:

* `abi.json` — the contract ABI, embedded via `//go:embed abi.json`
* `const EvmByteCode = "<hex>"` in `byte_code.go` — the deployed code returned by `eth_getCode`

Both are derived from a minimal Solidity **Caller** contract that implements the same interface the
precompile serves (e.g. `RUNECaller.sol` → `core/runetoken`). The Caller is a trick: any contract
that satisfies the precompile interface and matches its ABI works, and the callers are the minimal
such implementations.

`byte_code.go` is **generated code — do not hand-edit it.** Generate both files with:

```
# 1. compile the callers (requires npm deps installed in this directory)
npx hardhat compile

# 2. extract abi + deployedBytecode into the Go packages
./scripts/generate-bytecode.sh
```

Or, from the repository root:

```
make precompile-gen
```

The script strips the `0x` prefix (the Go constant must not contain it), validates that the value is
even-length hex, and writes the constant **wrapped in double quotes**:

```go
const EvmByteCode = "608060405234801561000f575f80fd5b..."   // correct
```

> ⚠️ **Why the script exists.** The manual process ("copy `deployedBytecode` from the artifact json
> and remove the `0x` prefix") is easy to get wrong. A hand-edit once dropped the surrounding quotes,
> producing `const EvmByteCode = 6080604052...`, which the Go compiler reads as bare tokens and
> rejects with `unexpected name ... after top level declaration`. Generating the file makes that
> impossible.

To add a new precompile, add its Caller contract to `core/precompile/contracts/`, then add a
`"<CallerName>:<gopackagedir>"` entry to the `MAPPINGS` array in `scripts/generate-bytecode.sh`.

### Running Tasks

Tasks get run as if they are built in hardhat commands. Read tasks are executed using a basic ethers provider
(no account), e.g:

```
npx hardhat --network testnet priceOracle:decimals
```

Write tasks all require at minimum a signer argument, e.g:

```
npx hardhat --network testnet runeToken:transfer --signer 0xc2f7Ae302a68CF215bb3dA243dadAB3290308015 --to 0x0 --value 1
```

## Localnet

If running against a local dev net a minor adjustment to the genesis files is currently required. Due to this you must
clean and re-initialize:

```
make localnet-bin-clean
make localnet-bin-init
make localnet-bin-start
...
```

The Hardhat configuration will automatically read and assign the localnet accounts generated by the commands above.
If you wish to override these accounts, you can run:

```
npx hardhat vars set RUNE_ACCOUNTS
```

where `RUNE_ACCOUNTS` are private keys separated by `,`.

# Custom precompile contract verification (blockscout)

## EvmByteCode

Generating the `EvmByteCode` used by a custom precompile can be done via `npx hardhat compile`. Look for the
`deployedBytecode` value in the contract artifact json file. Use this value as `EvmByteCode` - *make sure to remove
the 0x prefix*

The precompiles Caller contract is used for bytecode generation, technically this is a trick, we could use any contract
that satisfies the precompile's interface and matches the precompile's ABI. The caller contracts are used as they are
minimal implementations of the interfaces, and is semantically close to what's happening in reality.

## Source code

Flatten into a single solidity file to make verification easier, e.g:

`npx hardhat flatten contracts/RUNECaller.sol > ~/Desktop/RUNECaller.sol`

Use this file when verifying.
