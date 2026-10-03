import { RUNETransfers } from '../typechain-types/RUNETransfers';
import { expect } from "chai";
import hre from "hardhat";
import { ethers } from "hardhat"
import { getDeployedContract } from "./helpers/contract"
import btcabi from '../../../precompile/btctoken/abi.json'
import runeabi from '../../../precompile/runetoken/abi.json'

const btcPrecompileAddress = '0x7b7c000000000000000000000000000000000000';
const runePrecompileAddress = '0x7b7c000000000000000000000000000000000001';

describe("RUNETransfers", function () {
  const { deployments } = hre;
  let btcErc20Token: any;
  let runeErc20Token: any;
  let runeTransfers: RUNETransfers;
  let sender: any;
  let recipientAddress: string;

  const fixture = (async function () {
    await deployments.fixture(["RUNETransfers"]);
    btcErc20Token = new hre.ethers.Contract(btcPrecompileAddress, btcabi, ethers.provider);
    runeErc20Token = new hre.ethers.Contract(runePrecompileAddress, runeabi, ethers.provider);
    runeTransfers = await getDeployedContract("RUNETransfers");
    const signers = await ethers.getSigners();
    sender = signers[0];
    recipientAddress = ethers.Wallet.createRandom().address;
  });

  describe("transferSomeRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");

      const transferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await transferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).transferSomeRUNE(recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should not change recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance);
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("4"));
    });

    it("should not change contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance);
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("4"));
    });
  });

  describe("approveRUNEAndBTCSimultaneously", function () {
    let receipt: any;
    let tx: any;
    let receipient = ethers.Wallet.createRandom().address;

    before(async function () {
      await fixture();
    });

    // All the steps of this test are happening in the following `it` blocks.
    // Each of them will in turn execute an approve transaction and validate
    // that the state of the allowance between the receipient and sender
    // is correct accross multiple execution targeted at both the RUNE and
    // BTC token.

    it("should allow approving BTC for the receipient", async function () {
      tx =  await btcErc20Token.connect(sender)
        .approve(receipient, 10, {gasLimit: 1000000});
      await tx.wait();
      receipt = await ethers.provider.getTransactionReceipt(tx.hash);
      expect(receipt!.status).to.equal(1);
      const currentApproval = await btcErc20Token.allowance(sender.address, receipient);
      expect(currentApproval).to.equal(10);
    });

    it("should allow approving RUNE for the receipient", async function () {
      tx =  await runeErc20Token.connect(sender)
        .approve(receipient, 20, {gasLimit: 1000000});
      await tx.wait();
      receipt = await ethers.provider.getTransactionReceipt(tx.hash);
      expect(receipt!.status).to.equal(1);
      const currentRuneApproval = await runeErc20Token.allowance(sender.address, receipient);
      expect(currentRuneApproval).to.equal(20);
      const currentBTCApproval = await btcErc20Token.allowance(sender.address, receipient);
      expect(currentBTCApproval).to.equal(10);
    });

    it("should allow to update the approved amount of RUNE for the receipient", async function () {
      tx =  await runeErc20Token.connect(sender)
        .approve(receipient, 15, {gasLimit: 1000000});
      await tx.wait();
      receipt = await ethers.provider.getTransactionReceipt(tx.hash);
      expect(receipt!.status).to.equal(1);
      const currentRuneApproval = await runeErc20Token.allowance(sender.address, receipient);
      expect(currentRuneApproval).to.equal(15);
      const currentBTCApproval = await btcErc20Token.allowance(sender.address, receipient);
      expect(currentBTCApproval).to.equal(10);
    });

    it("should allow revoking the approved amount of BTC for the receipient", async function () {
      tx =  await btcErc20Token.connect(sender)
        .approve(receipient, 0, {gasLimit: 1000000});
      await tx.wait();
      receipt = await ethers.provider.getTransactionReceipt(tx.hash);
      expect(receipt!.status).to.equal(1);
      const currentBTCApproval = await btcErc20Token.allowance(sender.address, receipient);
      expect(currentBTCApproval).to.equal(0);
      const currentRuneApproval = await runeErc20Token.allowance(sender.address, receipient);
      expect(currentRuneApproval).to.equal(15);
    });

    it("should allow revoking the approved amount of RUNE for the receipient", async function () {
      // one first approve for a given value.
      tx =  await runeErc20Token.connect(sender)
        .approve(receipient, 0, {gasLimit: 1000000});
      await tx.wait();
      receipt = await ethers.provider.getTransactionReceipt(tx.hash);
      expect(receipt!.status).to.equal(1);
      const currentRuneApproval = await runeErc20Token.allowance(sender.address, receipient);
      expect(currentRuneApproval).to.equal(0);
      const currentBTCApproval = await btcErc20Token.allowance(sender.address, receipient);
      expect(currentBTCApproval).to.equal(0);
    });
  });


  describe("transferAllRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");

      const transferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await transferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).transferAllRUNE(recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should not change recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance);
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should not change contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance);
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("8"));
    });
  });

  describe("pullRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");

      const approveTx = await runeErc20Token.connect(sender).approve(runeTransfersAddress, runeAmount);
      await approveTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).pullRUNE(sender.address, runeAmount);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should properly decrease sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance - ethers.parseEther("8"));
    });

    it("should not change contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance);
    });

    it("should properly increase contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance + ethers.parseEther("8"));
    });
  });

  describe("pullRUNEToRecipient", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");

      const approveTx = await runeErc20Token.connect(sender).approve(runeTransfersAddress, runeAmount);
      await approveTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).pullRUNEToRecipient(sender.address, recipientAddress, runeAmount);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should properly decrease sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance - ethers.parseEther("8"));
    });

    it("should not change recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance);
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should not change contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance);
    });

    it("should not change contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance);
    });
  });

  describe("receiveNativeThenTransferRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const transferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await transferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).receiveNativeThenTransferRUNE(recipientAddress, {value: btcAmount});
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should properly decrease sender BTC balance", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - ethers.parseEther("6") - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should not change recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance);
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should properly increase contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance + ethers.parseEther("6"));
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("8"));
    });
  });

  describe("receiveNativeThenPullRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const approveTx = await runeErc20Token.connect(sender).approve(runeTransfersAddress, runeAmount);
      await approveTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).receiveNativeThenPullRUNE(sender.address, runeAmount, {value: btcAmount});
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should properly decrease sender BTC balance", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - ethers.parseEther("6") - gasCost);
    });

    it("should properly decrease sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance - ethers.parseEther("8"));
    });

    it("should properly increase contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance + ethers.parseEther("8"));
    });
  });

  describe("sendNativeThenTransferRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeTransferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await runeTransferTx.wait();

      const btcTransferTx = await btcErc20Token.connect(sender).transfer(runeTransfersAddress, btcAmount);
      await btcTransferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).sendNativeThenTransferRUNE(recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should properly increase recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should properly decrease contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance - ethers.parseEther("6"));
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("8"));
    });
  });

  describe("transferRUNEThenSendNative", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeTransferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await runeTransferTx.wait();

      const btcTransferTx = await btcErc20Token.connect(sender).transfer(runeTransfersAddress, btcAmount);
      await btcTransferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).transferRUNEThenSendNative(recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should properly increase recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should properly decrease contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance - ethers.parseEther("6"));
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("8"));
    });
  });

  describe("sendNativeThenPullRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeApproveTx = await runeErc20Token.connect(sender).approve(runeTransfersAddress, runeAmount);
      await runeApproveTx.wait();

      const btcTransferTx = await btcErc20Token.connect(sender).transfer(runeTransfersAddress, btcAmount);
      await btcTransferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).sendNativeThenPullRUNE(recipientAddress, sender.address, runeAmount);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should properly decrease sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance - ethers.parseEther("8"));
    });

    it("should properly increase recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance + ethers.parseEther("6"));
    });

    it("should not change recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance);
    });

    it("should properly decrease contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance - ethers.parseEther("6"));
    });

    it("should properly increase contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance + ethers.parseEther("8"));
    });
  });

  describe("pullRUNEThenSendNative", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeApproveTx = await runeErc20Token.connect(sender).approve(runeTransfersAddress, runeAmount);
      await runeApproveTx.wait();

      const btcTransferTx = await btcErc20Token.connect(sender).transfer(runeTransfersAddress, btcAmount);
      await btcTransferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).pullRUNEThenSendNative(sender.address, runeAmount, recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should properly decrease sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance - ethers.parseEther("8"));
    });

    it("should properly increase recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance + ethers.parseEther("6"));
    });

    it("should not change recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance);
    });

    it("should properly decrease contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance - ethers.parseEther("6"));
    });

    it("should properly increase contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance + ethers.parseEther("8"));
    });
  });

  describe("transferBTCThenTransferRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeTransferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await runeTransferTx.wait();

      const btcTransferTx = await btcErc20Token.connect(sender).transfer(runeTransfersAddress, btcAmount);
      await btcTransferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).transferBTCThenTransferRUNE(recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should properly increase recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should properly decrease contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance - ethers.parseEther("6"));
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("8"));
    });
  });

  describe("transferRUNEThenTransferBTC", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeTransferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await runeTransferTx.wait();

      const btcTransferTx = await btcErc20Token.connect(sender).transfer(runeTransfersAddress, btcAmount);
      await btcTransferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).transferRUNEThenTransferBTC(recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should properly increase recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should properly decrease contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance - ethers.parseEther("6"));
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("8"));
    });
  });

  describe("pullBTCThenPullRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeApproveTx = await runeErc20Token.connect(sender).approve(runeTransfersAddress, runeAmount);
      await runeApproveTx.wait();

      const btcApproveTx = await btcErc20Token.connect(sender).approve(runeTransfersAddress, btcAmount);
      await btcApproveTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).pullBTCThenPullRUNE(sender, btcAmount, runeAmount);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should properly decrease sender BTC balance", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - ethers.parseEther("6") - gasCost);
    });

    it("should properly decrease sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance - ethers.parseEther("8"));
    });

    it("should properly increase contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance + ethers.parseEther("8"));
    });
  });

  describe("pullRUNEThenPullBTC", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeApproveTx = await runeErc20Token.connect(sender).approve(runeTransfersAddress, runeAmount);
      await runeApproveTx.wait();

      const btcApproveTx = await btcErc20Token.connect(sender).approve(runeTransfersAddress, btcAmount);
      await btcApproveTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).pullRUNEThenPullBTC(sender, runeAmount, btcAmount);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should properly decrease sender BTC balance", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - ethers.parseEther("6") - gasCost);
    });

    it("should properly decrease sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance - ethers.parseEther("8"));
    });

    it("should properly increase contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance + ethers.parseEther("8"));
    });
  });

  describe("sendNativeThenTransferBTCThenTransferRUNE", function () {
    let runeTransfersAddress: string;
    let runeAmount: any;
    let btcAmount: any;

    let initialSenderRUNEBalance: any;
    let initialSenderBTCBalance: any;
    let initialRecipientRUNEBalance: any;
    let initialRecipientBTCBalance: any;
    let initialContractRUNEBalance: any;
    let initialContractBTCBalance: any;

    let gasCost: any;

    before(async function () {
      await fixture();

      runeTransfersAddress = await runeTransfers.getAddress();
      runeAmount = ethers.parseEther("8");
      btcAmount = ethers.parseEther("6");

      const runeTransferTx = await runeErc20Token.connect(sender).transfer(runeTransfersAddress, runeAmount);
      await runeTransferTx.wait();

      const btcTransferTx = await btcErc20Token.connect(sender).transfer(runeTransfersAddress, btcAmount);
      await btcTransferTx.wait();

      initialSenderRUNEBalance = await runeErc20Token.balanceOf(sender.address);
      initialSenderBTCBalance = await ethers.provider.getBalance(sender.address);

      initialRecipientRUNEBalance = await runeErc20Token.balanceOf(recipientAddress);
      initialRecipientBTCBalance = await ethers.provider.getBalance(recipientAddress);

      initialContractRUNEBalance = await runeErc20Token.balanceOf(runeTransfersAddress);
      initialContractBTCBalance = await ethers.provider.getBalance(runeTransfersAddress);

      const tx = await runeTransfers.connect(sender).sendNativeThenTransferBTCThenTransferRUNE(recipientAddress);
      const receipt = await tx.wait();
      gasCost = receipt.gasUsed * receipt.gasPrice;
    });

    it("should decrease sender BTC balance by gas cost", async function () {
        const current = await ethers.provider.getBalance(sender.address);
        expect(current).to.equal(initialSenderBTCBalance - gasCost);
    });

    it("should not change sender RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(sender.address);
        expect(current).to.equal(initialSenderRUNEBalance);
    });

    it("should properly increase recipient BTC balance", async function () {
        const current = await ethers.provider.getBalance(recipientAddress);
        expect(current).to.equal(initialRecipientBTCBalance + ethers.parseEther("6"));
    });

    it("should properly increase recipient RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(recipientAddress);
        expect(current).to.equal(initialRecipientRUNEBalance + ethers.parseEther("8"));
    });

    it("should properly decrease contract BTC balance", async function () {
        const current = await ethers.provider.getBalance(runeTransfersAddress);
        expect(current).to.equal(initialContractBTCBalance - ethers.parseEther("6"));
    });

    it("should properly decrease contract RUNE balance", async function () {
        const current = await runeErc20Token.balanceOf(runeTransfersAddress);
        expect(current).to.equal(initialContractRUNEBalance - ethers.parseEther("8"));
    });
  });
})
