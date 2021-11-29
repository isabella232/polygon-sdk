const { expect } = require("chai");
const { ethers } = require("hardhat");

const addr = "0x774ED5BcdF0aAA000cD9dCeAF3923271c7cd0b4B"

describe("Bridge", function () {
  it("Should return the new greeting once it's changed", async function () {
    const Bridge = await ethers.getContractFactory("Bridge");
    const bridge = await Bridge.deploy();
    await bridge.deployed();

    // emit and event and wait
    const eventTx = await bridge.emitEvent();
    const receipt = await eventTx.wait();

    console.log(receipt)

    //expect(await bridge.greet()).to.equal("Hola, mundo!");
  });
});

describe("Validator", function () {
  it("Should return the new greeting once it's changed", async function () {
    const Validator = await ethers.getContractFactory("Validator");
    const val = await Validator.deploy([addr]);
    await val.deployed();

    // set the validators
    await val.setValidators([addr])

    // check the validators
    const [validator] = await val.getValidators()
    expect(validator).to.equal(addr)
  });
});
