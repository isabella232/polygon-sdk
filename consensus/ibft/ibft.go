package ibft

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"path/filepath"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/consensus"
	"github.com/0xPolygon/polygon-sdk/consensus/polybft"
	"github.com/0xPolygon/polygon-sdk/contracts2"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/protocol"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/txpool"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	"github.com/umbracle/go-web3/wallet"
	"google.golang.org/grpc"
	goproto "google.golang.org/protobuf/proto"
)

const (
	DefaultEpochSize = 100000
)

type blockchainInterface interface {
	Header() *types.Header
	GetHeaderByNumber(i uint64) (*types.Header, bool)
	WriteBlocks(blocks []*types.Block) error
	CalculateGasLimit(number uint64) (uint64, error)
}

// Ibft represents the IBFT consensus mechanism object
type Ibft struct {
	sealing bool // Flag indicating if the node is a sealer

	logger hclog.Logger      // Output logger
	config *consensus.Config // Consensus configuration
	//state  *currentState     // Reference to the current state

	blockchain blockchainInterface // Interface exposed by the blockchain layer
	executor   *state.Executor     // Reference to the state executor
	closeCh    chan struct{}       // Channel for closing

	//validatorKey     *ecdsa.PrivateKey // Private key for the validator
	//validatorKeyAddr types.Address

	validatorKey *key

	txpool *txpool.TxPool // Reference to the transaction pool

	// we need to remove this to disable completely ibft
	// store     *snapshotStore // Snapshot store that keeps track of all snapshots
	epochSize uint64

	//msgQueue *msgQueue     // Structure containing different message queues
	//updateCh chan struct{} // Update channel

	syncer       *protocol.Syncer // Reference to the sync protocol
	syncNotifyCh chan bool        // Sync protocol notification channel

	network *network.Server // Reference to the networking layer

	// this should be the only things required up to this point
	polybft *polybft.PolyBFT
}

type key struct {
	priv *ecdsa.PrivateKey
	addr types.Address
}

func (k *key) String() string {
	return k.addr.String()
}

func (k *key) NodeID() pbft.NodeID {
	return pbft.NodeID(k.addr.String())
}

func (k *key) Sign(b []byte) ([]byte, error) {
	// TODO
	seal, err := crypto.Sign(k.priv, crypto.Keccak256(b))
	if err != nil {
		return nil, err
	}
	return seal, nil
}

func newKey(priv *ecdsa.PrivateKey) *key {
	k := &key{
		priv: priv,
		addr: crypto.PubKeyToAddress(&priv.PublicKey),
	}
	return k
}

// Factory implements the base consensus Factory method
func Factory(
	ctx context.Context,
	sealing bool,
	config *consensus.Config,
	txpool *txpool.TxPool,
	network *network.Server,
	blockchain *blockchain.Blockchain,
	executor *state.Executor,
	srv *grpc.Server,
	logger hclog.Logger,
) (consensus.Consensus, error) {
	p := &Ibft{
		logger:     logger.Named("ibft"),
		config:     config,
		blockchain: blockchain,
		executor:   executor,
		closeCh:    make(chan struct{}),
		txpool:     txpool,
		// state:        &currentState{},
		network:      network,
		epochSize:    DefaultEpochSize,
		syncNotifyCh: make(chan bool),
		sealing:      sealing,
	}

	// Istanbul requires a different header hash function
	types.HeaderHash = polybft.IstanbulHeaderHash

	p.syncer = protocol.NewSyncer(logger, network, blockchain)

	if err := p.createKey(); err != nil {
		return nil, err
	}

	p.logger.Info("validator key", "addr", p.validatorKey.NodeID())

	stdLogger := p.logger.StandardLogger(&hclog.StandardLoggerOptions{})

	addr, metadata := contracts2.GetRootChain()

	polybftConfig := &polybft.Config{
		JSONRPCEndpoint:       addr,
		BridgeAddr:            types.Address(metadata.Bridge),
		ValidatorContractAddr: contracts2.ValidatorContractAddr,
	}

	pp, err := polybft.NewPolyBFT(stdLogger, polybftConfig, p.config.Path, wallet.NewKey(p.validatorKey.priv), p, p, &wrapTransport{network})
	if err != nil {
		panic(err)
	}
	p.polybft = pp

	return p, nil
}

type wrapTransport struct {
	t *network.Server
}

func (w *wrapTransport) NewTopic(typ string, obj goproto.Message) (polybft.Topic, error) {
	t, err := w.t.NewTopic(typ, obj)
	if err != nil {
		return nil, err
	}
	return &wrapTopic{t}, nil
}

type wrapTopic struct {
	t *network.Topic
}

func (w *wrapTopic) Publish(obj goproto.Message) error {
	return w.t.Publish(obj)
}

func (w *wrapTopic) Subscribe(handler func(interface{})) error {
	w.t.Subscribe(handler)
	return nil
}

func (i *Ibft) Call(parent *types.Header, to types.Address, data []byte) ([]byte, error) {

	tt, err := i.executor.BeginTxn(parent.StateRoot, parent, types.Address{})
	if err != nil {
		panic(err)
	}

	txn := &types.Transaction{
		GasPrice: big.NewInt(100),
		Gas:      parent.GasLimit,
		To:       &contracts2.ValidatorContractAddr,
		Value:    big.NewInt(0),
		Input:    data,
	}
	res := tt.ApplyInt(parent.GasLimit, txn)
	return res.ReturnValue, nil
}

func (i *Ibft) Header() *types.Header {
	return i.blockchain.Header()
}

func (i *Ibft) InsertBlock(block *types.Block) error {
	if err := i.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
		return err
	}

	i.logger.Info(
		"block committed",
		//"sequence", i.state.view.Sequence,
		"hash", block.Hash(),
		//"validators", len(i.state.validators),
		//"rounds", i.state.view.Round+1,
		//"committed", i.state.numCommitted(),
	)

	// broadcast the new block
	i.syncer.Broadcast(block)

	// after the block has been written we reset the txpool so that
	// the old transactions are removed
	i.txpool.ResetWithHeader(block.Header)

	return nil
}

func (i *Ibft) IsStuck() (uint64, bool) {
	if i.syncer == nil {
		// we cannot answer it. skip it
		return 0, false
	}

	bestPeer := i.syncer.BestPeer()
	if bestPeer == nil {
		// there is no best peer, skip it
		return 0, false
	}

	lastProposal := i.blockchain.Header() // isnt this parent??
	if bestPeer.Number() > lastProposal.Number {
		// we are stuck
		return bestPeer.Number(), true
	}
	return 0, false
}

func (i *Ibft) BuildBlock(parent *types.Header, validators []types.Address, transactions []*polybft.StateTransaction) (*types.Block, error) {
	return i.buildBlock(parent, validators, transactions)
}

// Start starts the IBFT consensus
func (i *Ibft) Start() error {

	i.polybft.Setup()

	// Start the syncer
	i.syncer.Start()

	// Start the actual IBFT protocol
	go i.start()

	return nil
}

// createKey sets the validator's private key, from the file path
func (i *Ibft) createKey() error {
	// i.msgQueue = msgQueueImpl{}
	//i.msgQueue = newMsgQueue()
	i.closeCh = make(chan struct{})
	//i.updateCh = make(chan struct{})

	if i.validatorKey == nil {
		// generate a validator private key
		validatorKey, err := crypto.GenerateOrReadPrivateKey(filepath.Join(i.config.Path, IbftKeyName))
		if err != nil {
			return err
		}

		i.validatorKey = newKey(validatorKey)
		//i.validatorKey = validatorKey
		//i.validatorKeyAddr = crypto.PubKeyToAddress(&validatorKey.PublicKey)
	}

	return nil
}

const IbftKeyName = "validator.key"

/// --- FSM implementation of PoS ---

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

const epochSize = 10

func (i *Ibft) getValidators(parent *types.Header) []types.Address {
	// get a state reference for this to make calls and txns!
	// to call we need to use the parent reference

	xx, err := abi.NewABIFromList([]string{
		"function getValidators() returns (address[])",
	})
	if err != nil {
		panic(err)
	}

	tt, err := i.executor.BeginTxn(parent.StateRoot, parent, types.Address{})
	if err != nil {
		panic(err)
	}

	txn := &types.Transaction{
		GasPrice: big.NewInt(100),
		Gas:      parent.GasLimit,
		To:       &contracts2.ValidatorContractAddr,
		Value:    big.NewInt(0),
		Input:    xx.Methods["getValidators"].ID(),
	}
	res := tt.ApplyInt(parent.GasLimit, txn)

	raw, err := xx.Methods["getValidators"].Decode(res.ReturnValue)
	if err != nil {
		panic(err)
	}
	validators := raw["0"].([]web3.Address)

	finalRes := []types.Address{}
	for _, v := range validators {
		finalRes = append(finalRes, types.StringToAddress(v.String()))
	}
	return finalRes
}

// start starts the IBFT consensus state machine
func (i *Ibft) start() {
SYNC:
	// try to sync as much as possible
	i.runSyncState()

	for {
		parent := i.blockchain.Header()

		i.polybft.Run(parent)

		switch i.polybft.GetState() {
		case pbft.SyncState:
			// we need to go back to sync
			goto SYNC
		case pbft.DoneState:
			// everything worked, move to the next iteration
		default:
			// stopped
			return
		}
	}
}

// runSyncState implements the Sync state loop.
//
// It fetches fresh data from the blockchain. Checks if the current node is a validator and resolves any pending blocks
func (i *Ibft) runSyncState() {
	isSyncing := true

	isValidSnapshot := func() bool {
		if !i.isSealing() {
			return false
		}

		// check if we are a validator and enabled
		header := i.blockchain.Header()

		val := i.getValidators(header)
		dd := &dummySet{set: val, lastProposer: types.Address{}}

		if dd.Includes(pbft.NodeID(i.validatorKey.addr.String())) {
			// Do not need to do this since it will be done later
			return true
		}
		return false
	}

	for isSyncing {
		// try to sync with some target peer
		p := i.syncer.BestPeer()
		if p == nil {
			// if we do not have any peers and we have been a validator
			// we can start now. In case we start on another fork this will be
			// reverted later
			if isValidSnapshot() {
				// initialize the round and sequence
				// header := i.blockchain.Header()
				isSyncing = false
				/*
					i.state.view = &proto.View{
						Round:    0,
						Sequence: header.Number + 1,
					}
					i.setState(AcceptState)
				*/
			} else {
				time.Sleep(1 * time.Second)
			}
			continue
		}

		if err := i.syncer.BulkSyncWithPeer(p); err != nil {
			i.logger.Error("failed to bulk sync", "err", err)
			continue
		}

		// if we are a validator we do not even want to wait here
		// we can just move ahead
		if isValidSnapshot() {
			isSyncing = false
			continue
			//i.setState(AcceptState)
			//continue
		}

		// start watch mode
		var isValidator bool
		i.syncer.WatchSyncWithPeer(p, func(b *types.Block) bool {
			i.syncer.Broadcast(b)
			isValidator = isValidSnapshot()

			return isValidator
		})

		if isValidator {
			// at this point, we are in sync with the latest chain we know of
			// and we are a validator of that chain so we need to change to AcceptState
			// so that we can start to do some stuff there
			// i.setState(AcceptState)
			isSyncing = false
		}
	}
}

var defaultBlockPeriod = 2 * time.Second

// buildBlock builds the block, based on the passed in snapshot and parent header
func (i *Ibft) buildBlock(parent *types.Header, validators []types.Address, stateTxns []*polybft.StateTransaction) (*types.Block, error) {
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     parent.Number + 1,
		Miner:      types.Address{},
		Nonce:      types.Nonce{},
		//MixHash:    IstanbulDigest,
		Difficulty: parent.Number + 1,   // we need to do this because blockchain needs difficulty to organize blocks and forks
		StateRoot:  types.EmptyRootHash, // this avoids needing state for now
		Sha3Uncles: types.EmptyUncleHash,
		GasLimit:   parent.GasLimit, // Inherit from parent for now, will need to adjust dynamically later.
	}

	// calculate gas limit based on parent header
	gasLimit, err := i.blockchain.CalculateGasLimit(header.Number)
	if err != nil {
		return nil, err
	}
	header.GasLimit = gasLimit

	// set the timestamp
	parentTime := time.Unix(int64(parent.Timestamp), 0)
	headerTime := parentTime.Add(defaultBlockPeriod)

	if headerTime.Before(time.Now()) {
		headerTime = time.Now()
	}
	header.Timestamp = uint64(headerTime.Unix())

	transition, err := i.executor.BeginTxn(parent.StateRoot, header, types.Address{})
	if err != nil {
		return nil, err
	}
	txns := []*types.Transaction{}
	for {
		txn, retFn := i.txpool.Pop()
		if txn == nil {
			break
		}

		if txn.ExceedsBlockGasLimit(gasLimit) {
			i.logger.Error(fmt.Sprintf("failed to write transaction: %v", state.ErrBlockLimitExceeded))
			i.txpool.DecreaseAccountNonce(txn)
		} else {
			if err := transition.Write(txn); err != nil {
				if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable {
					retFn()
				} else {
					i.txpool.DecreaseAccountNonce(txn)
				}
				break
			}
			txns = append(txns, txn)
		}
	}
	i.logger.Info("picked out txns from pool", "num", len(txns), "remaining", i.txpool.Length())

	// a bit ugly but works
	// V3Notes: This does not take into account many things like:
	// 1. Do we have enough gas to make this post hooks?
	// 2. Should we send state txns first?
	// V3. Apply all the state transactions
	for _, txn := range stateTxns {
		transaction := &types.Transaction{
			Input:    txn.Input,
			To:       &txn.To,
			Value:    big.NewInt(0),
			GasPrice: big.NewInt(0),
		}
		if err := transition.Write(transaction); err != nil {
			panic(err)
		}
		txns = append(txns, transaction)
	}

	_, root := transition.Commit()
	header.StateRoot = root
	header.GasUsed = transition.TotalGas()

	i.logger.Info("final root", header.StateRoot)

	// build the block
	block := consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   header,
		Txns:     txns,
		Receipts: transition.Receipts(),
	})

	i.polybft.Finish(block, validators)

	/*
		// write the seal of the block after all the fields are completed
		header, err = writeSeal(i.validatorKey.priv, block.Header)
		if err != nil {
			return nil, err
		}
		block.Header = header
	*/

	// compute the hash, this is only a provisional hash since the final one
	// is sealed after all the committed seals
	block.Header.ComputeHash()

	i.logger.Info("build block", "number", header.Number, "txns", len(txns))
	return block, nil
}

// isSealing checks if the current node is sealing blocks
func (i *Ibft) isSealing() bool {
	return i.sealing
}

// verifyHeaderImpl implements the actual header verification logic
func (i *Ibft) verifyHeaderImpl(parent, header *types.Header) error {

	if err := i.polybft.VerifyHeader(header); err != nil {
		return err
	}
	if header.Sha3Uncles != types.EmptyUncleHash {
		return fmt.Errorf("invalid sha3 uncles")
	}

	// difficulty has to match number
	if header.Difficulty != header.Number {
		return fmt.Errorf("wrong difficulty")
	}

	/*
		// V3NOTE: RESTORE
		// verify the sealer
		if err := verifySigner(snap, header); err != nil {
			return err
		}
	*/

	return nil
}

// VerifyHeader wrapper for verifying headers
func (i *Ibft) VerifyHeader(parent, header *types.Header) error {
	// verify all the header fields + seal
	if err := i.verifyHeaderImpl(parent, header); err != nil {
		return err
	}

	/*
		// V3NOTE: Restore
		// verify the commited seals
		if err := verifyCommitedFields(header); err != nil {
			return err
		}
	*/

	return nil
}

// GetBlockCreator retrieves the block signer from the extra data field
func (i *Ibft) GetBlockCreator(header *types.Header) (types.Address, error) {
	return i.polybft.BlockCreator(header)
}

// Close closes the IBFT consensus mechanism, and does write back to disk
func (i *Ibft) Close() error {
	close(i.closeCh)

	if i.config.Path != "" {
		/*
			err := i.store.saveToPath(i.config.Path)

			if err != nil {
				return err
			}
		*/
	}
	return nil
}
