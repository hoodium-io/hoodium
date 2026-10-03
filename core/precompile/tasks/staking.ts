import { task } from 'hardhat/config'
import '@nomicfoundation/hardhat-toolbox'

import abi from '../../staking/abi.json'
const precompileAddress = '0x19be000000000000000000000000000000000003'

task('staking:getDelegation', 'Returns the RUNE amount delegated by `delegator` to `validator`')
  .addParam('delegator', 'EVM address of the delegator')
  .addParam('validator', 'EVM address of the validator operator')
  .setAction(async (taskArguments, hre) => {
    const staking = new hre.ethers.Contract(precompileAddress, abi, hre.ethers.provider)
    const amount = await staking.getDelegation(taskArguments.delegator, taskArguments.validator)
    console.log(amount)
  })

task('staking:delegate', 'Delegates `value` RUNE from the signer to `validator`')
  .addParam('signer', 'The signer address (msg.sender)')
  .addParam('validator', 'EVM address of the validator operator')
  .addParam('value', 'Value to delegate (arune)')
  .setAction(async (taskArguments, hre) => {
    const signer = await hre.ethers.getSigner(taskArguments.signer)
    const staking = new hre.ethers.Contract(precompileAddress, abi, signer)
    const pending = await staking.delegate(taskArguments.validator, taskArguments.value)
    const confirmed = await pending.wait()
    console.log(confirmed.hash)
  })

task('staking:undelegate', 'Undelegates `value` RUNE from `validator`')
  .addParam('signer', 'The signer address (msg.sender)')
  .addParam('validator', 'EVM address of the validator operator')
  .addParam('value', 'Value to undelegate (arune)')
  .setAction(async (taskArguments, hre) => {
    const signer = await hre.ethers.getSigner(taskArguments.signer)
    const staking = new hre.ethers.Contract(precompileAddress, abi, signer)
    const pending = await staking.undelegate(taskArguments.validator, taskArguments.value)
    const confirmed = await pending.wait()
    console.log(confirmed.hash)
  })