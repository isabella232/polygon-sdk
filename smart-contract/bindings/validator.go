// Code generated by go-web3/abigen. DO NOT EDIT.
// Hash: 467024471dc5050578eb63c13e0ea7209039a91f66a2589e4f77bf74ed4cf170
package bindings

import (
	"fmt"
	"math/big"

	web3 "github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/contract"
	"github.com/umbracle/go-web3/jsonrpc"
)

var (
	_ = big.NewInt
)

// Validator is a solidity contract
type Validator struct {
	c *contract.Contract
}

// DeployValidator deploys a new Validator contract
func DeployValidator(provider *jsonrpc.Client, from web3.Address, args ...interface{}) *contract.Txn {
	return contract.DeployContract(provider, from, abiValidator, binValidator, args...)
}

// NewValidator creates a new instance of the contract at a specific address
func NewValidator(addr web3.Address, provider *jsonrpc.Client) *Validator {
	return &Validator{c: contract.NewContract(addr, abiValidator, provider)}
}

// Contract returns the contract object
func (v *Validator) Contract() *contract.Contract {
	return v.c
}

// calls

// GetValidators calls the getValidators method in the solidity contract
func (v *Validator) GetValidators(block ...web3.BlockNumber) (retval0 []web3.Address, err error) {
	var out map[string]interface{}
	var ok bool

	out, err = v.c.Call("getValidators", web3.EncodeBlock(block...))
	if err != nil {
		return
	}

	// decode outputs
	retval0, ok = out["_validators"].([]web3.Address)
	if !ok {
		err = fmt.Errorf("failed to encode output at index 0")
		return
	}
	
	return
}

// txns

// SetValidators sends a setValidators transaction in the solidity contract
func (v *Validator) SetValidators(validators []web3.Address) *contract.Txn {
	return v.c.Txn("setValidators", validators)
}

// UpdateValidatorSet sends a updateValidatorSet transaction in the solidity contract
func (v *Validator) UpdateValidatorSet(data []byte) *contract.Txn {
	return v.c.Txn("updateValidatorSet", data)
}

// events
