// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import { IHOODI } from "../../hooditoken/IHOODI.sol";

contract HOODICaller is IHOODI {
    address private constant precompile = 0x19be000000000000000000000000000000000001;

    function name() external view returns (string memory) {
        return IHOODI(precompile).name();
    }

    function symbol() external view returns (string memory) {
        return IHOODI(precompile).symbol();
    }

    function decimals() external view returns (uint8) {
        return IHOODI(precompile).decimals();
    }

    function totalSupply() external view returns (uint256) {
        return IHOODI(precompile).totalSupply();
    }

    function maxSupply() external view returns (uint256) {
        return IHOODI(precompile).maxSupply();
    }

    function balanceOf(address account) external view returns (uint256) {
        return IHOODI(precompile).balanceOf(account);
    }

    function transfer(address to, uint256 value) external returns (bool) {
        return IHOODI(precompile).transfer(to, value);
    }

    function allowance(address owner, address spender) external view returns (uint256) {
        return IHOODI(precompile).allowance(owner, spender);
    }

    function approve(address spender, uint256 value) external returns (bool) {
        return IHOODI(precompile).approve(spender, value);
    }

    function transferFrom(address from, address to, uint256 value) external returns (bool) {
        return IHOODI(precompile).transferFrom(from, to, value);
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
        return IHOODI(precompile).permit(owner, spender, amount, deadline, v, r, s);
    }

    function DOMAIN_SEPARATOR() external view returns (bytes32) {
        return IHOODI(precompile).DOMAIN_SEPARATOR();
    }

    // Deprecated as it is not compatible with EIP-2612.
    // Should be removed in the future.
    function nonce(address owner) external view returns (uint256) {
        return IHOODI(precompile).nonce(owner);
    }

    function nonces(address owner) external view returns (uint256) {
        return IHOODI(precompile).nonces(owner);
    }

    function PERMIT_TYPEHASH() external pure returns (bytes32) {
        return IHOODI(precompile).PERMIT_TYPEHASH();
    }
}