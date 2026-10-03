// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import { IStaking } from "../interfaces/IStaking.sol";

contract StakingCaller is IStaking {
    address private constant precompile = 0x19be000000000000000000000000000000000003;

    function delegate(address validator, uint256 amount) external returns (bool) {
        return IStaking(precompile).delegate(validator, amount);
    }

    function undelegate(address validator, uint256 amount) external returns (bool) {
        return IStaking(precompile).undelegate(validator, amount);
    }

    function getDelegation(address delegator, address validator) external view returns (uint256) {
        return IStaking(precompile).getDelegation(delegator, validator);
    }
}