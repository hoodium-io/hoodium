// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

import {IRUNE} from "./interfaces/IRUNE.sol";
import {IBTC} from "./interfaces/IBTC.sol";

/// @title RUNETransfers
/// @notice Handles various transfer scenarios for the RUNE token.
contract RUNETransfers {
    // RUNE token address on Rune.
    address private constant runePrecompile = 0x7B7c000000000000000000000000000000000001;
    // BTC token address on Rune.
    address private constant btcPrecompile = 0x7b7C000000000000000000000000000000000000;

    /// @notice Transfers half of the contract's RUNE balance to the recipient.
    function transferSomeRUNE(address recipient) external {
        uint256 balance = IRUNE(runePrecompile).balanceOf(address(this));
        require(balance > 0, "No balance to transfer");

        uint256 halfBalance = balance / 2;

        bool success = IRUNE(runePrecompile).transfer(recipient, halfBalance);
        require(success, "Transfer failed");
    }

    /// @notice Transfers the entire contract's RUNE balance to the recipient.
    function transferAllRUNE(address recipient) external {
        uint256 balance = IRUNE(runePrecompile).balanceOf(address(this));
        require(balance > 0, "No balance to transfer");

        bool success = IRUNE(runePrecompile).transfer(recipient, balance);
        require(success, "Transfer failed");
    }

    /// @notice Pulls the given amount of RUNE from the sender to the contract.
    function pullRUNE(address sender, uint256 amount) external {
        bool success = IRUNE(runePrecompile).transferFrom(sender, address(this), amount);
        require(success, "Transfer failed");
    }

    /// @notice Pulls the given amount of RUNE from the sender to the given recipient.
    function pullRUNEToRecipient(address sender, address recipient, uint256 amount) external {
        bool success = IRUNE(runePrecompile).transferFrom(sender, recipient, amount);
        require(success, "Transfer failed");
    }

    /// @notice Receives native BTC (msg.value) and transfers the contract's 
    ///         RUNE balance to the recipient.
    function receiveNativeThenTransferRUNE(address recipient) external payable {
        require(msg.value > 0, "Must send some native BTC");

        uint256 runeBalance = IRUNE(runePrecompile).balanceOf(address(this));
        require(runeBalance > 0, "No RUNE balance to transfer");

        bool success = IRUNE(runePrecompile).transfer(recipient, runeBalance);
        require(success, "Transfer failed");
    }

    /// @notice Receives native BTC (msg.value) and pulls the given amount of 
    ///         RUNE from the sender to the contract.
    function receiveNativeThenPullRUNE(address sender, uint256 amount) external payable {
        require(msg.value > 0, "Must send some native BTC");

        bool success = IRUNE(runePrecompile).transferFrom(sender, address(this), amount);
        require(success, "Transfer failed");
    }

    /// @notice Sends the entire contract's BTC balance natively then transfers 
    ///         the contract's RUNE balance to the recipient.
    function sendNativeThenTransferRUNE(address recipient) external {
        uint256 btcBalance = address(this).balance;
        require(btcBalance > 0, "No BTC balance to send");

        (bool sent,) = recipient.call{value: btcBalance}("");
        require(sent, "Native transfer failed");

        uint256 runeBalance = IRUNE(runePrecompile).balanceOf(address(this));
        require(runeBalance > 0, "No RUNE balance to transfer");

        bool success = IRUNE(runePrecompile).transfer(recipient, runeBalance);
        require(success, "Transfer failed");
    }

    /// @notice Transfers the contract's RUNE balance then sends the entire contract's 
    ///         BTC balance natively to the recipient.
    function transferRUNEThenSendNative(address recipient) external {
        uint256 runeBalance = IRUNE(runePrecompile).balanceOf(address(this));
        require(runeBalance > 0, "No RUNE balance to transfer");

        bool success = IRUNE(runePrecompile).transfer(recipient, runeBalance);
        require(success, "Transfer failed");

        uint256 btcBalance = address(this).balance;
        require(btcBalance > 0, "No BTC balance to send");

        (bool sent,) = recipient.call{value: btcBalance}("");
        require(sent, "Native transfer failed");
    }

    /// @notice Sends the contract's BTC balance natively to the recipient then pulls 
    ///         the given RUNE amount from the given sender.
    function sendNativeThenPullRUNE(address btcRecipient, address runeSender, uint256 runeAmount) external {
        uint256 btcBalance = address(this).balance;
        require(btcBalance > 0, "No BTC balance to send");

        (bool sent,) = btcRecipient.call{value: btcBalance}("");
        require(sent, "Native transfer failed");

        bool success = IRUNE(runePrecompile).transferFrom(runeSender, address(this), runeAmount);
        require(success, "Transfer failed");
    }

    /// @notice Pulls the given amount of RUNE from the sender then sends the entire 
    ///         contract's BTC balance natively to the recipient.
    function pullRUNEThenSendNative(address runeSender, uint256 runeAmount, address btcRecipient) external {
        bool success = IRUNE(runePrecompile).transferFrom(runeSender, address(this), runeAmount);
        require(success, "Transfer failed");

        uint256 btcBalance = address(this).balance;
        require(btcBalance > 0, "No BTC balance to send");

        (bool sent,) = btcRecipient.call{value: btcBalance}("");
        require(sent, "Native transfer failed");
    }

    /// @notice Transfers the contract's BTC balance (using ERC20 precompile) then transfers 
    ///         the contract's RUNE balance to the recipient.
    function transferBTCThenTransferRUNE(address recipient) external {
        uint256 btcBalance = IBTC(btcPrecompile).balanceOf(address(this));
        require(btcBalance > 0, "No BTC balance to transfer");

        bool success1 = IBTC(btcPrecompile).transfer(recipient, btcBalance);
        require(success1, "Transfer failed");

        uint256 runeBalance = IRUNE(runePrecompile).balanceOf(address(this));
        require(runeBalance > 0, "No RUNE balance to transfer");

        bool success2 = IRUNE(runePrecompile).transfer(recipient, runeBalance);
        require(success2, "Transfer failed");
    }

    /// @notice Transfers the contract's RUNE balance then transfers the contract's BTC 
    ///         balance (using ERC20 precompile) to the recipient.
    function transferRUNEThenTransferBTC(address recipient) external {
        uint256 runeBalance = IRUNE(runePrecompile).balanceOf(address(this));
        require(runeBalance > 0, "No RUNE balance to transfer");

        bool success1 = IRUNE(runePrecompile).transfer(recipient, runeBalance);
        require(success1, "Transfer failed");

        uint256 btcBalance = IBTC(btcPrecompile).balanceOf(address(this));
        require(btcBalance > 0, "No BTC balance to transfer");

        bool success2 = IBTC(btcPrecompile).transfer(recipient, btcBalance);
        require(success2, "Transfer failed");
    }

    /// @notice Pulls the given amount of BTC from the sender then pulls the given amount of 
    ///         RUNE from the sender.
    function pullBTCThenPullRUNE(address sender, uint256 btcAmount, uint256 runeAmount) external {
        bool success1 = IBTC(btcPrecompile).transferFrom(sender, address(this), btcAmount);
        require(success1, "Transfer failed");

        bool success2 = IRUNE(runePrecompile).transferFrom(sender, address(this), runeAmount);
        require(success2, "Transfer failed");
    }
    
    /// @notice Pulls the given amount of RUNE from the sender then pulls the given amount of 
    ///         BTC from the sender.
    function pullRUNEThenPullBTC(address sender, uint256 runeAmount, uint256 btcAmount) external {
        bool success1 = IRUNE(runePrecompile).transferFrom(sender, address(this), runeAmount);
        require(success1, "Transfer failed");

        bool success2 = IBTC(btcPrecompile).transferFrom(sender, address(this), btcAmount);
        require(success2, "Transfer failed");
    }

    /// @notice Sends half of the contract's BTC balance natively then transfers another half
    ///         using the BTC ERC20 precompile then transfers the contract's RUNE balance 
    ///         to the recipient.
    function sendNativeThenTransferBTCThenTransferRUNE(address recipient) external {
        uint256 btcBalance = address(this).balance;
        require(btcBalance > 0, "No BTC balance to send");

        uint256 halfBalance = btcBalance / 2;

        (bool sent,) = recipient.call{value: halfBalance}("");
        require(sent, "Native transfer failed");    

        bool success1 = IBTC(btcPrecompile).transfer(recipient, halfBalance);
        require(success1, "Transfer failed");

        uint256 runeBalance = IRUNE(runePrecompile).balanceOf(address(this));
        require(runeBalance > 0, "No RUNE balance to transfer");

        bool success2 = IRUNE(runePrecompile).transfer(recipient, runeBalance);
        require(success2, "Transfer failed");
    }
}
