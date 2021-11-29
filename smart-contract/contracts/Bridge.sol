//SPDX-License-Identifier: Unlicense
pragma solidity ^0.8.0;

import "hardhat/console.sol";

contract Bridge {
    event Transfer();

    function emitEvent() public {
        emit Transfer();
    }
    
    // it is done because of an small error in the Go abigen function
    function dummy() public view returns (uint256) {
        return 0;
    }
}
