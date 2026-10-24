// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import { IStaking } from "../interfaces/IStaking.sol";

contract StakingCaller is IStaking {
    address private constant precompile = 0x19be000000000000000000000000000000000003;

    function delegate(address validator, uint256 amount) external returns (bool) {
        return IStaking(precompile).delegate(validator, amount);
    }

    function createValidator(
        bytes calldata consensusPubKey,
        uint256 amount,
        string calldata moniker,
        uint256 commissionRate,
        uint256 commissionMaxRate,
        uint256 commissionMaxChangeRate,
        uint256 minSelfDelegation
    ) external returns (bool) {
        return IStaking(precompile).createValidator(
            consensusPubKey,
            amount,
            moniker,
            commissionRate,
            commissionMaxRate,
            commissionMaxChangeRate,
            minSelfDelegation
        );
    }

    function unjail(address validator) external returns (bool) {
        return IStaking(precompile).unjail(validator);
    }

    function undelegate(address validator, uint256 amount) external returns (bool) {
        return IStaking(precompile).undelegate(validator, amount);
    }

    function getDelegation(address delegator, address validator) external view returns (uint256) {
        return IStaking(precompile).getDelegation(delegator, validator);
    }
}