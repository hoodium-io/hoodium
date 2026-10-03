// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import { IRUNE } from "../../runetoken/IRUNE.sol";

contract RUNECaller is IRUNE {
    address private constant precompile = 0x19be000000000000000000000000000000000000;

    function name() external view returns (string memory) {
        return IRUNE(precompile).name();
    }

    function symbol() external view returns (string memory) {
        return IRUNE(precompile).symbol();
    }

    function decimals() external view returns (uint8) {
        return IRUNE(precompile).decimals();
    }

    function totalSupply() external view returns (uint256) {
        return IRUNE(precompile).totalSupply();
    }

    function maxSupply() external view returns (uint256) {
        return IRUNE(precompile).maxSupply();
    }

    function balanceOf(address account) external view returns (uint256) {
        return IRUNE(precompile).balanceOf(account);
    }

    function transfer(address to, uint256 value) external returns (bool) {
        return IRUNE(precompile).transfer(to, value);
    }

    function allowance(address owner, address spender) external view returns (uint256) {
        return IRUNE(precompile).allowance(owner, spender);
    }

    function approve(address spender, uint256 value) external returns (bool) {
        return IRUNE(precompile).approve(spender, value);
    }

    function transferFrom(address from, address to, uint256 value) external returns (bool) {
        return IRUNE(precompile).transferFrom(from, to, value);
    }

    function permit(
        address owner,
        address spender,
        uint256 amount,
        uint256 deadline,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external returns (bool) {
        return IRUNE(precompile).permit(owner, spender, amount, deadline, v, r, s);
    }

    function DOMAIN_SEPARATOR() external view returns (bytes32) {
        return IRUNE(precompile).DOMAIN_SEPARATOR();
    }

    // Deprecated as it is not compatible with EIP-2612.
    // Should be removed in the future.
    function nonce(address owner) external view returns (uint256) {
        return IRUNE(precompile).nonce(owner);
    }

    function nonces(address owner) external view returns (uint256) {
        return IRUNE(precompile).nonces(owner);
    }

    function PERMIT_TYPEHASH() external pure returns (bytes32) {
        return IRUNE(precompile).PERMIT_TYPEHASH();
    }
}