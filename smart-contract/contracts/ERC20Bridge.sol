// contracts/GLDToken.sol
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

interface ERC20BridgeToken {
   function mintTo(address to, uint256 amount) external payable;
}

contract ERC20Bridge {
    ERC20BridgeToken target;

    constructor(ERC20BridgeToken _target) public {
        target = _target;
    }

    function stateSync(address to, uint256 amount) public payable {
        target.mintTo(to, amount);
    }
}

// THIS IS NOT BEING USED RIGHT NOW, FOR SIMPLICITY WE WILL ADD THE STATE SYNC FUNCTION IN THE TOKEN ITSELF
// FOR MODULARITY, ON PRODUCTION WE HAVE A WRAPPER BUT SINCE IN THIS TESTING FRAMEWORK IT IS CUMBERSOME TO CREATE
// CONTRACTS WE WILL ONLY DEPLOY ONE.
