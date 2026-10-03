// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title  IStaking
/// @notice Interface for the Staking precompile.
/// @dev    Exposes the RUNE delegation surface to EVM wallets so a user can
///         delegate to, and undelegate from, a validator without switching to a
///         Cosmos wallet. Validator addresses are the EVM (20-byte) form of the
///         validator operator address. Amounts are expressed in the RUNE base
///         denomination (arune).
interface IStaking {
    /**
     * @notice Delegates `amount` RUNE from the caller to the given validator.
     * @param validator The EVM address of the validator operator.
     * @param amount    The amount to delegate, in arune (RUNE base denomination).
     * @return True on success.
     */
    function delegate(address validator, uint256 amount) external returns (bool);

    /**
     * @notice Undelegates `amount` RUNE from the given validator.
     * @param validator The EVM address of the validator operator.
     * @param amount    The amount to undelegate, in arune (RUNE base denomination).
     * @return True on success.
     */
    function undelegate(address validator, uint256 amount) external returns (bool);

    /**
     * @notice Returns the RUNE amount delegated by `delegator` to `validator`.
     * @param delegator The EVM address of the delegator.
     * @param validator The EVM address of the validator operator.
     * @return The delegated amount, in arune (RUNE base denomination).
     */
    function getDelegation(address delegator, address validator) external view returns (uint256);
}