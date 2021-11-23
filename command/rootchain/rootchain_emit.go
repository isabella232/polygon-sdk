package rootchain

import (
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/contracts2"
	"github.com/0xPolygon/polygon-sdk/smart-contract/bindings"
	"github.com/mitchellh/cli"
	"github.com/umbracle/go-web3/jsonrpc"
)

// RootchainEmitCommand is the command to show the version of the agent
type RootchainEmitCommand struct {
	UI cli.Ui
	helper.Meta
}

// Help implements the cli.Command interface
func (c *RootchainEmitCommand) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *RootchainEmitCommand) Synopsis() string {
	return ""
}

// Run implements the cli.Command interface
func (c *RootchainEmitCommand) Run(args []string) int {
	addr, metadata := contracts2.GetRootChain()

	provider, err := jsonrpc.NewClient(addr)
	if err != nil {
		panic(err)
	}
	owner, err := provider.Eth().Accounts()
	if err != nil {
		panic(err)
	}

	/*
		nonce, err := provider.Eth().GetNonce(owner[0], web3.Latest)
		if err != nil {
			panic(err)
		}
		hash, err := provider.Eth().SendTransaction(&web3.Transaction{
			Gas:      5242880,
			GasPrice: 1879048192,
			From:     owner[0],
			Input:    contracts2.BridgeBin(),
			To:       &metadata.Bridge,
			Nonce:    nonce,
		})

		time.Sleep(2 * time.Second)
		receipt, err := provider.Eth().GetTransactionReceipt(hash)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Done: %s\n", receipt.TransactionHash)

		fmt.Println(receipt)
		fmt.Println(receipt.Logs)
	*/

	bridge := bindings.NewBridge(metadata.Bridge, provider)
	bridge.Contract().SetFrom(owner[0])

	txn := bridge.SetGreeting("xxx")
	if err := txn.DoAndWait(); err != nil {
		panic(err)
	}

	fmt.Println(txn.Receipt())
	fmt.Println(txn.Receipt().Logs)

	/*
		fmt.Println(txn.Receipt().ContractAddress)

		fmt.Println("- get code -")
		fmt.Println(provider.Eth().GetCode(txn.Receipt().ContractAddress, web3.Latest))

		cc := bindings.NewBridge(txn.Receipt().ContractAddress, provider)
		cc.Contract().SetFrom(owner[0])

		txn = cc.SetGreeting("Xxxx")
		if err := txn.DoAndWait(); err != nil {
			panic(err)
		}

		fmt.Println(txn.Receipt())
		fmt.Println(txn.Receipt().Logs)
	*/

	return 0
}
