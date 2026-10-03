// SPDX-License-Identifier: MIT

pragma solidity ^0.8.20;

import "./erc20/IERC20WithPermit.sol";

/// @title  IHOODI
/// @notice Interface for the HOODI token.
interface IHOODI is IERC20WithPermit {
    /// @notice Returns the hard cap on the total supply of the token.
    function maxSupply() external view returns (uint256);
}
