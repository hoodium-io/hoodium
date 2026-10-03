// SPDX-License-Identifier: GPL-3.0

pragma solidity 0.8.29;

import "./RUNE.sol";

/// @title Cross-chain RUNE Deployer
/// @notice RUNEDeployer allows deploying the RUNE token on another EVM chain at a deterministic address.
///         The RUNEDeployer itself should be deployed on the chain via the EIP-2470 singleton factory.
/// @dev Based on: https://github.com/mezo-org/musd/blob/main/solidity/contracts/token/TokenDeployer.sol
contract RUNEDeployer {
    bytes32 public constant SALT =
        keccak256("Bank on yourself. Bring everyday finance to your Bitcoin.");

    /// @notice The governance address receiving the control over the token;
    /// @dev This is the same multisig as the one used to control protocol parameters of the chain.
    address public constant GOVERNANCE = 0x98D8899c3030741925BE630C710A98B57F397C7a;    

    /// @notice The address of the deployed RUNE token contract.
    /// @dev Zero address before the contract is deployed.
    address public token;

    event TokenDeployed(address token);

    error TokenAlreadyDeployed();

    /// @notice Deploys the RUNE token to the chain via create2, and initiates 
    ///         the 2-step ownership transfer process to the governance.
    /// @dev IMPORTANT NOTES: 
    ///      - This method can be called by anyone
    ///      - The initial owner of the deployed token is this deployer contract
    ///      - The initial minters and burners are unset
    ///      - The governance must finalize the 2-step ownership transfer process 
    ///        by issuing an acceptance transaction
    ///      - The governance must set the minters and burners according to the needs
    function deployToken() external {
        if (token != address(0)) {
            revert TokenAlreadyDeployed();
        }

        // Deploy the RUNE contract using CREATE2.
        // Note that {salt: SALT} is syntactic sugar assembling a CREATE2 call under the hood.
        token = address(new RUNE{salt: SALT}());

        emit TokenDeployed(token);
        
        // Initiate the 2-step ownership transfer process to the governance.
        RUNE(token).transferOwnership(GOVERNANCE);
    }
}
