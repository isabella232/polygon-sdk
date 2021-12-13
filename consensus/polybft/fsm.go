package polybft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
)

var stateSyncEvent = abi.MustNewEvent(`event Transfer(address token, address to, uint256 amount)`)

// StateTransaction is a special type of transaction that was not sent as part of the normal
// txpool but agreed as part of the consensus protocol by all the validators. Note that this
// transactions do not include a From address and should skip any balance or pre-check.
type StateTransaction struct {
	To    types.Address
	Input []byte
}

type ProposedBlock struct {
	Raw  []byte
	Hash []byte
}

// BlockBuilder is the instance that we run inside the fsm
type BlockBuilder interface {
	// BuildBlock is executed by the proposer to gather transactions and build the block
	BuildBlock(timestamp time.Time, stateTransactions []*StateTransaction) (*ProposedBlock, error)

	// TODO: Executed by non-proposers to validate a block proposal. This only involves transaction execution
	Validate(proposal []byte) ([]byte, error)

	// Commit is the final stage to effectively write the block
	Commit(extra []byte)
}

// This should be the interface we require from the outside
type Backend interface {
	// BlockBuilder returns a block builder
	BlockBuilder(parent *types.Header) BlockBuilder

	// Header returns the current header of the blockchain
	Header() *types.Header

	// IsStuck returns wether the block is out of sync (name might be confusing)
	IsStuck() (uint64, bool)
}

type fsm2 struct {
	p *PolyBFT
	b Backend

	// this is the hash of the block, the thing we sign
	hash []byte

	seal []byte

	stateTransactions []*StateTransaction
	parent            *types.Header
	validators        []types.Address
	lastProposer      types.Address

	builder BlockBuilder
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

var (
	defaultBlockPeriod = 2 * time.Second
)

// polyBFTProposal is the proposal we are trying to commit with polybft
type polyBFTProposal struct {
	BlockRaw []byte

	// We could move this field to pbft.Proposal?
	Seal []byte

	// We could move this field to pbft.Proposal?
	Hash []byte
}

func (p *polyBFTProposal) Marshal() ([]byte, error) {
	// HACK(ferran). this is not the best marshal
	return json.Marshal(p)
}

func (p *polyBFTProposal) Unmarshal(b []byte) error {
	return json.Unmarshal(b, p)
}

func (f *fsm2) BuildProposal() (*pbft.Proposal, error) {
	// THIS IS PART OF ACCETSTATE, THE PART THAT WE RUN IF WE ARE THE PROPOSER
	f.builder = f.b.BlockBuilder(f.parent)

	// set timestamp (selecting the timestamp is a config of the consensus protocol)
	parentTime := time.Unix(int64(f.parent.Timestamp), 0)
	headerTime := parentTime.Add(defaultBlockPeriod)

	if headerTime.Before(time.Now()) {
		headerTime = time.Now()
	}

	proposal, err := f.builder.BuildBlock(headerTime, f.stateTransactions)
	if err != nil {
		return nil, err
	}
	f.hash = proposal.Hash

	// add the seal to the block
	seal, err := writeSeal2(f.p.key, proposal.Hash, false)
	if err != nil {
		return nil, err
	}
	f.seal = seal

	polyBFTProposal := &polyBFTProposal{
		BlockRaw: proposal.Raw,
		Seal:     seal,
		Hash:     f.hash,
	}
	data, err := polyBFTProposal.Marshal()
	if err != nil {
		panic(err)
	}

	proposal2 := &pbft.Proposal{
		Time: time.Unix(int64(headerTime.Unix()), 0),
		Data: data,
	}
	return proposal2, nil
}

func (f *fsm2) Validate(proposal []byte) error {

	// decode the proposal
	polybftProposal := &polyBFTProposal{}
	if err := polybftProposal.Unmarshal(proposal); err != nil {
		return err
	}
	f.seal = polybftProposal.Seal

	// THIS IS PART OF ACCETSTATE, THE PART WE RUN DURING THE VALIDATION
	// AT THIS POINT WE SHOULD GET THE PROPOSAL OBJECT TOO.
	f.builder = f.b.BlockBuilder(f.parent)
	hash, err := f.builder.Validate(polybftProposal.BlockRaw)
	if err != nil {
		panic(err)
	}
	if !bytes.Equal(polybftProposal.Hash, hash) {
		// we have been notified of a hash that is not correct
		panic("bad")
	}
	f.hash = hash

	// TODO: Validate seal as well? or do that in pbft?
	return nil
}

func (f *fsm2) Insert(p *pbft.SealedProposal) error {
	// THIS DATA IS ALREADY IN AS PART OF THE VALIDATION STEP AND
	// THE ACCEPT STATE
	// IDEALLY, VALIDATION SHOULD ALREADY WRITE THE DATA AND VALIDATE ROOTS
	// THEN, AT THIS POINT IT IS ONLY A MATTER OF INCLUDING THE COMMITED SEALS AND FINISH
	// THIS IS, UPDATE THE EXTRA?

	extra := &Extra{
		Seal:          f.seal,
		Validators:    f.validators,
		CommittedSeal: p.CommittedSeals,
	}

	// fill in 32 since the extra starts after 32 bytes (vanity extra)
	f.builder.Commit(extra.MarshalRLPTo(make([]byte, 32)))

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

	msg := commitMsg(f.hash)
	return msg
}

func (f *fsm2) IsStuck(num uint64) (uint64, bool) {
	return f.b.IsStuck()
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
