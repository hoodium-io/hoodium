
import { DeployFunction } from "hardhat-deploy/dist/types"
import { HardhatRuntimeEnvironment } from "hardhat/types"
import { deployWithSingletonFactory } from "../helpers/erc2470"

const func: DeployFunction = async (hre: HardhatRuntimeEnvironment) => {
  const { ethers, helpers, deployments } = hre
  const { log } = deployments
  const { deployer } = await helpers.signers.getNamedSigners()

  const existingDeployment = await deployments.getOrNull("RUNEDeployer")
  const isValidDeployment = existingDeployment &&
    helpers.address.isValid(existingDeployment.address)

  if (isValidDeployment) {
    log(`Using RUNEDeployer at ${existingDeployment.address}`)
  } else {
    log("Deploying the RUNEDeployer...")

    const deployTx = await deployWithSingletonFactory(
      hre,
      "RUNEDeployer",
      {
        contractName: "contracts/RUNEDeployer.sol:RUNEDeployer",
        from: deployer,
        salt: ethers.keccak256(
          // Note that this is the salt for deploying the RUNEDeployer contract.
          // The salt for RUNE token contract is defined inside the RUNEDeployer
          // as a SALT constant. Both salts doesn't have to be the same but we keep
          // them as such for consistency.
          ethers.toUtf8Bytes(
            "Bank on yourself. Bring everyday finance to your Bitcoin.",
          ),
        ),
        confirmations: 12,
      },
    )

    if (hre.network.tags.verify) {
      await helpers.etherscan.verify(deployTx.deployment)
    }
  }
}

export default func

func.tags = ["RUNEDeployer"]
