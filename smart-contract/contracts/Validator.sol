//SPDX-License-Identifier: Unlicense
pragma solidity ^0.8.0;

import "hardhat/console.sol";

contract Validator {
    // As of now we are going to use the same thing for both
    // bridge and validators
    // list of current validators
    address[] validators;

    constructor(address[] memory _validators) public {
        validators = _validators;
    }

    function setValidators(address[] memory _validators) public payable {
        validators = _validators;
    }

    function getValidators() public view returns (address[] memory _validators) {
        return validators;
    }
}
