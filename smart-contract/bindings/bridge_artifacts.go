package bindings

import (
	"encoding/hex"
	"fmt"

	"github.com/umbracle/go-web3/abi"
)

var abiBridge *abi.ABI

// BridgeAbi returns the abi of the Bridge contract
func BridgeAbi() *abi.ABI {
	return abiBridge
}

var binBridge []byte

// BridgeBin returns the bin of the Bridge contract
func BridgeBin() []byte {
	return binBridge
}

func init() {
	var err error
	abiBridge, err = abi.NewABI(abiBridgeStr)
	if err != nil {
		panic(fmt.Errorf("cannot parse Bridge abi: %v", err))
	}
	if len(binBridgeStr) != 0 {
		binBridge, err = hex.DecodeString(binBridgeStr[2:])
		if err != nil {
			panic(fmt.Errorf("cannot parse Bridge bin: %v", err))
		}
	}
}

var binBridgeStr = "0x608060405234801561001057600080fd5b5060f28061001f6000396000f3fe6080604052348015600f57600080fd5b506004361060325760003560e01c806332e43a111460375780637b0cb839146051575b600080fd5b603d6059565b604051604891906099565b60405180910390f35b6057605e565b005b600090565b7f406dade31f7ae4b5dbc276258c28dde5ae6d5c2773c5745802c493a2360e55e060405160405180910390a1565b60938160b2565b82525050565b600060208201905060ac6000830184608c565b92915050565b600081905091905056fea264697066735822122096a11cc15347ff7e99004198d5f09ef25cf7856bad4fdcef3c121a12cf2216f564736f6c63430008040033"

var abiBridgeStr = `[
    {
      "anonymous": false,
      "inputs": [],
      "name": "Transfer",
      "type": "event"
    },
    {
      "inputs": [],
      "name": "dummy",
      "outputs": [
        {
          "internalType": "uint256",
          "name": "",
          "type": "uint256"
        }
      ],
      "stateMutability": "view",
      "type": "function"
    },
    {
      "inputs": [],
      "name": "emitEvent",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    }
  ]`
