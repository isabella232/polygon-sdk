const { expect } = require("chai");
const { ethers } = require("hardhat");

describe("Bridge", function () {
  it("Should return the new greeting once it's changed", async function () {
    const Bridge = await ethers.getContractFactory("Bridge");
    const bridge = await Bridge.deploy("Hello, world!");
    await bridge.deployed();

    expect(await bridge.greet()).to.equal("Hello, world!");

    const setGreetingTx = await bridge.setGreeting("Hola, mundo!");

    // wait until the transaction is mined
    const receipt = await setGreetingTx.wait();

    console.log(receipt)

    //expect(setGreetingTx).to.emit("Transfer")
    //setGreetingTx.to.emit(eventEmitter, "Transfer")

    expect(await bridge.greet()).to.equal("Hola, mundo!");
  });
});
