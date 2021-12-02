//SPDX-License-Identifier: Unlicense
pragma solidity ^0.8.0;

import "hardhat/console.sol";

contract Bridge {
    event Transfer(address token, address to, uint256 amount);

    function emitEvent(address token, address to, uint256 amount) public {
        emit Transfer(token, to, amount);
    }
    
    // it is done because of an small error in the Go abigen function
    function dummy() public view returns (uint256) {
        return 0;
    }
}
