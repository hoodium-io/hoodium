// SPDX-License-Identifier: LGPL-3.0
// Copyright 2022 Evmos Foundation
// This file is part of the Evmos Network packages.
//
// Evmos is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Evmos packages are distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Evmos packages. If not, see https://github.com/evmos/evmos/blob/main/LICENSE
pragma solidity 0.8.29;

import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Permit.sol";
import "@openzeppelin/contracts/access/Ownable2Step.sol";

// @title RUNE
// @notice Cross-chain representation of the RUNE token.
// @dev RUNE token is a native precompile of the chain.
//      This contract serves as a representation of the RUNE
//      token on foreign EVM chains for bridging purposes.
contract BTC is ERC20Permit, Ownable2Step {
    /// @notice Addresses authorized to mint tokens.
    mapping(address => bool) public minters;
    /// @notice Addresses authorized to burn tokens.
    mapping(address => bool) public burners;

    /// @notice Emitted when a minter is set/unset.
    event MinterSet(address indexed minter, bool state);
    /// @notice Emitted when a burner is set/unset.
    event BurnerSet(address indexed burner, bool state);

    error NotMinter();
    error NotBurner();

    constructor() ERC20("RUNE", "RUNE") ERC20Permit("RUNE") Ownable(_msgSender()) {}

    /// @notice Mints `amount` tokens to `account`.
    /// @param account The address to mint tokens to.
    /// @param amount The amount of tokens to mint.
    /// @dev Throws NotMinter if the caller is not a minter.
    function mint(address account, uint256 amount) external {
        if (!minters[_msgSender()]) {
            revert NotMinter();
        }
        _mint(account, amount);
    }

    /// @notice Burns `amount` tokens from the caller.
    /// @param amount The amount of tokens to burn.
    /// @dev Throws NotBurner if the caller is not a burner.
    function burn(uint256 amount) external {
        if (!burners[_msgSender()]) {
            revert NotBurner();
        }
        _burn(_msgSender(), amount);
    }

    /// @notice Sets/unsets a minter.
    /// @param minter The address of the minter.
    /// @param state The new state of the minter (true to set, false to unset).
    /// @dev Throws OwnableUnauthorizedAccount if the caller is not the owner.
    function setMinter(address minter, bool state) public onlyOwner {
        minters[minter] = state;
        emit MinterSet(minter, state);
    }

    /// @notice Sets/unsets a burner.
    /// @param burner The address of the burner.
    /// @param state The new state of the burner (true to set, false to unset).
    /// @dev Throws OwnableUnauthorizedAccount if the caller is not the owner.
    function setBurner(address burner, bool state) public onlyOwner {
        burners[burner] = state;
        emit BurnerSet(burner, state);
    }
}
