package polybft

import (
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
)

var stateSyncEvent = abi.MustNewEvent(`event Transfer(address token, address to, uint256 amount)`)

type StateTransaction struct {
	To    types.Address
	Input []byte
}

// This should be the interface we require from the outside
type Backend interface {
	InsertBlock(b *types.Block) error
	Header() *types.Header
	IsStuck() (uint64, bool)
	BuildBlock(parent *types.Header, validators []types.Address, transactions []*StateTransaction) (*types.Block, error)
	Hash(p []byte) ([]byte, error)
}

type fsm2 struct {
	p *PolyBFT
	b Backend

	// this is the hash of the block, the thing we sign
	hash []byte

	stateTransactions []*StateTransaction
	parent            *types.Header
	validators        []types.Address
	lastProposer      types.Address
}

func (f *fsm2) init() error {
	f.validators = f.p.getValidators(f.parent) // this should be done before

	fmt.Println("-- vv --")
	fmt.Println(f.validators)

	var err error

	var lastProposer types.Address
	if f.parent.Number != 0 {
		lastProposer, err = ecrecoverFromHeader(f.parent)
		if err != nil {
			return err
		}
	}
	f.lastProposer = lastProposer

	if err := f.buildStateTransactions(); err != nil {
		panic(err)
	}
	return nil
}

func (f *fsm2) buildStateTransactions() error {
	f.stateTransactions = []*StateTransaction{}

	if !f.isEndOfEpoch() {
		return nil
	}

	// V3NOTE: If we are at the end of the epoch we try to:
	// 1. fit as many state sync as possible from the pool
	// Create a transaction with the item, this will be special state transaction (for now)
	// that does not check the sender.
	for _, msg := range f.p.pool.GetReady() {
		// convert msg into log
		var log web3.Log
		if err := log.UnmarshalJSON(msg.Data); err != nil {
			panic(err)
		}

		fmt.Println("__ MSG __")
		fmt.Println(msg)
		fmt.Println(log)

		// convert log into a transaction
		vals, err := stateSyncEvent.ParseLog(&log)
		if err != nil {
			panic(err)
		}
		fmt.Println("-- vals --")
		fmt.Println(vals)

		// address token, address to, uint256 amount
		tokenAddrRaw := vals["token"].(web3.Address)
		toAddr := vals["to"].(web3.Address)
		amount := vals["amount"].(*big.Int)

		// V3NOTE: THIS ONLY WORKS FOR NOW FOR ERC20 TOKENS. IT IS TRIVIAL TO DO IT FOR ARBITRARY VALUES LATER.
		// This works but the abi from the abigen dont, figure it out. I think it has to be with the ... ternary ops
		method, err := abi.NewMethod("function stateSync(address to, uint256 amount)")
		if err != nil {
			panic(err)
		}
		input, err := method.Encode(map[string]interface{}{
			"to":     toAddr,
			"amount": amount,
		})
		if err != nil {
			panic(err)
		}

		tokenAddr := types.StringToAddress(tokenAddrRaw.String())

		/*
			transaction := &types.Transaction{
				Input:    input,
				To:       &tokenAddr,
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(0),
			}
		*/

		fmt.Printf("---> STATE SYNC CONTRACT: %s %s %d\n", tokenAddr, toAddr, amount)

		// V3NOTE: IMPORTANT FOR THIS THINGS TO USE WRITE() BECAUSE THIS FUNCTION WILL INTERNALLY HANDLE EVERYTHING
		// OF STATESYNC FUNCTIONS. OTHERWISE, YOU MIGHT MISS SOME THINGS THAT ARE PART OF THE CONSENSUS AS WELL.
		//if err := v.Write(transaction); err != nil {
		//	panic(err)
		//}
		f.stateTransactions = append(f.stateTransactions, &StateTransaction{
			Input: input,
			To:    tokenAddr,
		})
	}

	// 2. update the validator set.
	{
		method, err := abi.NewMethod("function updateValidatorSet(bytes data)")
		if err != nil {
			// we can do this better
			panic(err)
		}
		input, err := method.Encode(map[string]interface{}{
			"data": []byte{},
		})
		if err != nil {
			panic(err)
		}
		/*
			transaction := &types.Transaction{
				Input:    input,
				To:       &contracts2.ValidatorContractAddr,
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(0),
			}
		*/
		fmt.Println("---> UPDATE VALIDATOR SET <---")

		//	if err := v.Write(transaction); err != nil {
		//		panic(err)
		//	}
		f.stateTransactions = append(f.stateTransactions, &StateTransaction{
			Input: input,
			To:    f.p.config.ValidatorContractAddr,
		})
	}
	return nil
}

func (f *fsm2) isEndOfEpoch() bool {
	return f.Height()%10 == 0
}

func (f *fsm2) BuildProposal() (*pbft.Proposal, error) {
	// THIS IS PART OF ACCETSTATE, THE PART THAT WE RUN DURING THE PROPOSAL

	// SEVERAL WAYS TO APPROACH THE PROBLEM OF SENDING CUSTOM TXNS TO THE CLIENT
	// 1. WE EITHER LET THE PROPOSER CREATE THE TXNS AND SEND THEM TO THE VALIDATORS
	// 	  THEN, EACH ONE SHOULD VALIDATE EACH OF THOSE CUSTOM TRANSACTIONS (I.E. IS THIS MESSAGE POOL GOOD?)
	//    (I.E. DOES IT INCLUDES VALIDATORSETCHANGE AT THE END OF EPOCH?)
	// 2. WE ONLY PASS THE BLOCK TRANSACTIONS AND EACH VALIDATOR ON ITS OWN INCLUDES THE TXNS AND BLOCKS
	//    THE ONLY PROBLEM HERE IS THAT WE ARE LOCKING A STRANGE HASH IN PBFT AND WE WILL ONLY KNOW AFTER
	// 	  THE FINAL BLOCK IS INCLUDE IS SOMETHING WENT WRONG. (THIS IS, STATE TXNS ARE NOT PART OF THE PBFT PROTOCOL).
	// 3. A MIX OF BOTH, WE HAVE A GENERIC FUNCTION CALLED GETSTATETXNS() THAT RETURNS ALL THE STATE TRANSACTIONS
	//    DURING THE VALIDATION STAGE, WE CHECK THAT ALL THIS TRANSACTIONS ARE INCLUDED IN THE LOCKED BLOCK.

	block, err := f.b.BuildBlock(f.parent, f.validators, f.stateTransactions)
	if err != nil {
		panic(err)
	}

	data := block.MarshalRLP()
	proposal := &pbft.Proposal{
		Time: time.Unix(int64(block.Header.Timestamp), 0),
		Data: data,
	}

	// block builder comes here..
	// this later on is differetnw ith the builder
	// 1. get the hash
	// 2. sign it
	// 3. add the extra
	// 4. include the hash in the proposal
	hash, err := f.b.Hash(data)
	if err != nil {
		panic(err)
	}
	f.hash = hash
	// 2. sign it and include validators

	return proposal, nil
}

func (f *fsm2) Validate(proposal []byte) error {
	// THIS IS PART OF ACCETSTATE, THE PART WE RUN DURING THE VALIDATION
	// AT THIS POINT WE SHOULD GET THE PROPOSAL OBJECT TOO.

	hash, err := f.b.Hash(proposal)
	if err != nil {
		panic(err)
	}
	f.hash = hash

	// TODO: we need to validate the state transactions
	// This is hard to do though since at th

	return nil
}

func (f *fsm2) Insert(p *pbft.SealedProposal) error {
	// THIS DATA IS ALREADY IN AS PART OF THE VALIDATION STEP AND
	// THE ACCEPT STATE
	// IDEALLY, VALIDATION SHOULD ALREADY WRITE THE DATA AND VALIDATE ROOTS
	// THEN, AT THIS POINT IT IS ONLY A MATTER OF INCLUDING THE COMMITED SEALS AND FINISH
	// THIS IS, UPDATE THE EXTRA?

	block := &types.Block{}
	if err := block.UnmarshalRLP(p.Proposal); err != nil {
		panic(err)
	}

	header, err := writeCommittedSeals(block.Header, p.CommittedSeals)
	if err != nil {
		return err
	}

	// we need to recompute the hash since we have change extra-data
	block.Header = header
	block.Header.ComputeHash()

	/// TODO: InsertBlock()
	if err := f.b.InsertBlock(block); err != nil {
		return err
	}

	/*
		if err := f.i.insertBlock(block, p.CommittedSeals); err != nil {
			panic(err)
		}
	*/

	return nil
}

func (f *fsm2) Height() uint64 {
	return f.parent.Number + 1
}

func (f *fsm2) ValidatorSet() pbft.ValidatorSet {
	dd := &dummySet{set: f.validators, lastProposer: f.lastProposer}
	return dd
}

func (f *fsm2) Hash(p []byte) []byte {
	// THIS IS THE COMMIT SEAL

	/*
		hash, err := f.b.Hash(p)
		if err != nil {
			panic(err)
		}
	*/

	/*
		block := &types.Block{}
		if err := block.UnmarshalRLP(p); err != nil {
			panic(err)
		}
		hash, err := calculateHeaderHash(block.Header)
		if err != nil {
			panic(err)
		}
	*/

	//fmt.Println("-- hash --")
	//fmt.Println(block.Header)
	//fmt.Println(hash)

	msg := commitMsg(f.hash)

	//fmt.Println("-- msg --")
	//fmt.Println(msg)

	return msg
}

func (f *fsm2) IsStuck(num uint64) (uint64, bool) {
	return f.b.IsStuck()

	/*
		if f.i.syncer == nil {
			// we cannot answer it. skip it
			return 0, false
		}

		bestPeer := f.i.syncer.BestPeer()
		if bestPeer == nil {
			// there is no best peer, skip it
			return 0, false
		}

		lastProposal := f.i.blockchain.Header() // isnt this parent??
		if bestPeer.Number() > lastProposal.Number {
			// we are stuck
			return bestPeer.Number(), true
		}
		return 0, false
	*/
}

type dummySet struct {
	lastProposer types.Address
	set          ValidatorSet
}

func (d *dummySet) CalcProposer(round uint64) pbft.NodeID {
	proposer := d.set.CalcProposer(round, d.lastProposer)
	return pbft.NodeID(proposer.String())
}

func (d *dummySet) Includes(id pbft.NodeID) bool {
	for _, i := range d.set {
		if i.String() == string(id) {
			return true
		}
	}
	return false
}

func (d *dummySet) Len() int {
	return d.set.Len()
}
