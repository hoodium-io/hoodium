# Hoodium Token Contracts

Solidity contracts that are inherent components of native features provided by the
Hoodium chain client. They represent Hoodium's native tokens on **foreign EVM chains**
for cross-chain/bridging purposes.

## Contracts

| Contract | Token | Purpose |
|----------|-------|---------|
| `HOODI.sol` | HOODI | Cross-chain representation of the HOODI token (native precompile on Hoodium). |
| `HOODIDeployer.sol` | HOODI | Deploys `HOODI` on a foreign chain at a deterministic address via the EIP-2470 singleton factory. |
| `RUNE.sol` | RUNE | Cross-chain representation of the RUNE token (native precompile on Hoodium). |
| `RUNEDeployer.sol` | RUNE | Deploys `RUNE` on a foreign chain at a deterministic address via the EIP-2470 singleton factory. |

## Notes

- These contracts are deployed **on foreign chains** (not on Hoodium itself). On Hoodium,
  RUNE and HOODI are native precompiles.
- The deployers are meant to be deployed through the EIP-2470 singleton factory so the
  resulting token addresses are deterministic across chains.
- Deployment scripts live in `deploy/`.
