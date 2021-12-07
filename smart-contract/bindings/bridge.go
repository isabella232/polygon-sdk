// Code generated by go-web3/abigen. DO NOT EDIT.
// Hash: 620c8fa6ca3dc427c936d048887f3514012a369a6e693637f27d964293c31784
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

// Bridge is a solidity contract
type Bridge struct {
	c *contract.Contract
}

// DeployBridge deploys a new Bridge contract
func DeployBridge(provider *jsonrpc.Client, from web3.Address, args ...interface{}) *contract.Txn {
	return contract.DeployContract(provider, from, abiBridge, binBridge, args...)
}

// NewBridge creates a new instance of the contract at a specific address
func NewBridge(addr web3.Address, provider *jsonrpc.Client) *Bridge {
	return &Bridge{c: contract.NewContract(addr, abiBridge, provider)}
}

// Contract returns the contract object
func (b *Bridge) Contract() *contract.Contract {
	return b.c
}

// calls

// Dummy calls the dummy method in the solidity contract
func (b *Bridge) Dummy(block ...web3.BlockNumber) (retval0 *big.Int, err error) {
	var out map[string]interface{}
	var ok bool

	out, err = b.c.Call("dummy", web3.EncodeBlock(block...))
	if err != nil {
		return
	}

	// decode outputs
	retval0, ok = out["0"].(*big.Int)
	if !ok {
		err = fmt.Errorf("failed to encode output at index 0")
		return
	}
	
	return
}

// txns

// EmitEvent sends a emitEvent transaction in the solidity contract
func (b *Bridge) EmitEvent(token web3.Address, to web3.Address, amount *big.Int) *contract.Txn {
	return b.c.Txn("emitEvent", token, to, amount)
}

// events

func (b *Bridge) TransferEventSig() web3.Hash {
	return b.c.ABI().Events["Transfer"].ID()
}