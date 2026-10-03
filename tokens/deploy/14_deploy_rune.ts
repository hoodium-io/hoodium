import type { HardhatRuntimeEnvironment } from "hardhat/types"
import type { DeployFunction } from "hardhat-deploy/types"
import { saveDeploymentArtifact, waitForTransaction } from "../helpers/deploy-helpers"

const func: DeployFunction = async function (hre: HardhatRuntimeEnvironment) {
  const { deployments, getNamedAccounts, helpers } = hre
  const { execute, read, log } = deployments
  const { deployer } = await getNamedAccounts()

  const existingDeployment = await deployments.getOrNull("RUNE")
  const isValidDeployment = existingDeployment &&
    helpers.address.isValid(existingDeployment.address)

  if (isValidDeployment) {
    log(`Using RUNE at ${existingDeployment.address}`)
    return
  }

  const deployTx = await execute(
    "RUNEDeployer",
    { from: deployer, log: true },
    "deployToken",
  )

  await waitForTransaction(hre, deployTx.transactionHash, 12)

  const RUNEAddress = await read("RUNEDeployer", "token")

  log(`RUNE deployed at ${RUNEAddress}`)

  const deployment = await saveDeploymentArtifact(
    hre,
    "RUNE",
    RUNEAddress,
    deployTx.transactionHash,
    {
      log: true,
    },
  )

  if (hre.network.tags.verify) {
    await helpers.etherscan.verify(deployment, "contracts/RUNE.sol:RUNE")
  }
}

export default func

func.tags = ["RUNE"]
func.dependencies = ["RUNEDeployer"]
