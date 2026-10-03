import { vars, HardhatUserConfig } from 'hardhat/config'
import { ethers } from 'ethers'
import '@nomicfoundation/hardhat-toolbox'
import * as dotenv from "dotenv";
// import precompile tasks
import './tasks/runetoken'
import './tasks/hooditoken'
import './tasks/util'
import './tasks/priceoracle'
import './tasks/staking'
import fs from 'fs'
import path from 'path'

const BUILD_DIR = '../../.localnet/'
const COUNT = 4

function getPrivKeys (): string[] {
  const strings: string[] = vars.get('RUNE_ACCOUNTS', '').split(',')
  const keys: string[] = []
  if (strings[0] !== '') {
    // RUNE accounts have been set already
    for (const str of strings) {
      if (str !== '') {
        keys.push(str)
      }
    }
  } else {
    // Fall back to reading localnet key_seed.json files. Skip missing files so
    // `hardhat compile` (which needs no accounts) does not fail before a localnet
    // has been bootstrapped.
    for (let i = 0; i < COUNT; i++) {
      const filePath = path.resolve(`${BUILD_DIR}node${i}/runed/key_seed.json`)
      if (!fs.existsSync(filePath)) {
        continue
      }
      const seed = JSON.parse(fs.readFileSync(filePath, 'utf8'))
      const pk: string = ethers.Wallet.fromPhrase(seed.secret).privateKey
      keys.push(pk)
    }
  }

  return keys
}

// Load .env file
dotenv.config();

const config: HardhatUserConfig = {
  solidity: {
    version: '0.8.24',
    settings: {
      optimizer: {
        enabled: true,
        runs: 200
      },
      evmVersion: 'cancun'
    }
  },
  defaultNetwork: 'localhost',
  networks: {
    localhost: {
      url: 'http://localhost:8545',
      chainId: 6591,
      accounts: getPrivKeys(),
      gas: 'auto'
    },
    testnet: {
      chainId: 6591,
      url: process.env.TESTNET_RPC_URL || "",
      accounts: process.env.TESTNET_PRIVATE_KEY ? [process.env.TESTNET_PRIVATE_KEY] : [],
    },
    mainnet: {
      chainId: 6590,
      url: process.env.MAINNET_RPC_URL || "",
      accounts: process.env.MAINNET_PRIVATE_KEY ? [process.env.MAINNET_PRIVATE_KEY] : [],
    }
  }
}

export default config
