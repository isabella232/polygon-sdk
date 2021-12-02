const { expect } = require("chai");
const { ethers } = require("hardhat");

const addr = "0x774ED5BcdF0aAA000cD9dCeAF3923271c7cd0b4B"

describe("Bridge", function () {
  it("Should return the new greeting once it's changed", async function () {
    const [owner] = await ethers.getSigners()

    const Bridge = await ethers.getContractFactory("Bridge");
    const bridge = await Bridge.deploy();
    await bridge.deployed();

    // emit and event and wait
    const eventTx = await bridge.emitEvent(owner.address, owner.address, 1);
    const receipt = await eventTx.wait();

    //console.log(receipt)
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

describe("ERC20MintToken", function(){
  it("Should mint the token", async function() {
    const [owner] = await ethers.getSigners()

    const Token = await ethers.getContractFactory("MintERC20")
    const token = await Token.deploy()

    console.log(await token.balanceOf(owner.address))

    await token.mint(100)

    console.log(await token.balanceOf(owner.address))
  })
})

describe("ERC20BridgeWrapper", function() {
  it("Should update the token balance upon state sync", async function() {
    const [owner] = await ethers.getSigners()

    const Token = await ethers.getContractFactory("MintERC20")
    const Wrapper = await ethers.getContractFactory("ERC20Bridge")

    const token = await Token.deploy()
    const wrapper = await Wrapper.deploy(token.address)

    // now call state sync in wrapper and the balance of token should change
    await wrapper.stateSync(owner.address, 100)

    console.log(await token.balanceOf(owner.address))
  })
})
