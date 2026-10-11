# Validator Setup

How to join an **official, already-running** Hoodium network as a validator.

> Running a node for the first time? Start with [Node Setup](./node-setup.md),
> which covers `runed setup` and the two bootstrap modes.
>
> Launching a new network's genesis? That is a team-only action — see
> [Mainchain Genesis Initialization](./team/mainchain/genesis-init.md).

## Joining an already-running network

Validators join **from their own nodes**, without any action from the bootstrap
operator:

1. They obtain the **official `genesis.json`** (same file everyone uses — its hash
   identifies the network).
2. They run `init` with the **same chain-id** and the shared genesis.
3. They fund their own wallet and submit:
   ```
   runed tx staking create-validator \
     --amount 500000000000000000000000arune \
     --pubkey "$(runed tendermint show-validator)" \
     --moniker <moniker> --commission-rate 0.05 \
     --min-self-delegation 500000000000000000000000 \
     --from <key-name> --chain-id <chain-id> --home <home>
   ```

Only the validator's **public consensus key** is recorded in genesis
(`validators[]`); the private `priv_validator_key.json` never leaves the node.

The bootstrap validator has **no special role** once other validators are bonded:
if it goes down, the remaining validators keep the chain running (provided less
than one third of the voting power is offline).

## Staking parameters (Hoodium)

| Parameter | Value |
|-----------|-------|
| Max validators | 101 |
| Min self-delegation | 500,000 RUNE |
| Min commission rate | 5% |

> A minimum **delegation** of 0.1 RUNE is enforced at the message/precompile level
> (not a staking param).