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

var binBridgeStr = "0x608060405234801561001057600080fd5b50610243806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c806332e43a111461003b578063d4232b7a14610059575b600080fd5b610043610075565b6040516100509190610188565b60405180910390f35b610073600480360381019061006e91906100e4565b61007a565b005b600090565b7fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef8383836040516100ad93929190610151565b60405180910390a1505050565b6000813590506100c9816101df565b92915050565b6000813590506100de816101f6565b92915050565b6000806000606084860312156100f957600080fd5b6000610107868287016100ba565b9350506020610118868287016100ba565b9250506040610129868287016100cf565b9150509250925092565b61013c816101a3565b82525050565b61014b816101d5565b82525050565b60006060820190506101666000830186610133565b6101736020830185610133565b6101806040830184610142565b949350505050565b600060208201905061019d6000830184610142565b92915050565b60006101ae826101b5565b9050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b6101e8816101a3565b81146101f357600080fd5b50565b6101ff816101d5565b811461020a57600080fd5b5056fea2646970667358221220a42ffcba351b3455c7203457aed3ba112efb5853140266e900da6d0bd8238a6164736f6c63430008040033"

var abiBridgeStr = `[
    {
      "anonymous": false,
      "inputs": [
        {
          "indexed": false,
          "internalType": "address",
          "name": "token",
          "type": "address"
        },
        {
          "indexed": false,
          "internalType": "address",
          "name": "to",
          "type": "address"
        },
        {
          "indexed": false,
          "internalType": "uint256",
          "name": "amount",
          "type": "uint256"
        }
      ],
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
      "inputs": [
        {
          "internalType": "address",
          "name": "token",
          "type": "address"
        },
        {
          "internalType": "address",
          "name": "to",
          "type": "address"
        },
        {
          "internalType": "uint256",
          "name": "amount",
          "type": "uint256"
        }
      ],
      "name": "emitEvent",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    }
  ]`
