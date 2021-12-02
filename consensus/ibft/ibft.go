package ibft

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"path/filepath"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	"github.com/umbracle/go-web3/jsonrpc"
	"github.com/umbracle/go-web3/tracker"

	boltdbStore "github.com/umbracle/go-web3/tracker/store/boltdb"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/consensus"
	pool "github.com/0xPolygon/polygon-sdk/consensus/ibft/message_pool"
	"github.com/0xPolygon/polygon-sdk/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-sdk/contracts2"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/protocol"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/txpool"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
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

	network   *network.Server // Reference to the networking layer
	transport transport       // Reference to the transport protocol

	operator *operator

	pbft *pbft.Pbft

	// aux test methods
	// forceTimeoutCh bool
	pool *pool.MessagePool // this is for state sync right now
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
	types.HeaderHash = istanbulHeaderHash

	p.syncer = protocol.NewSyncer(logger, network, blockchain)

	// register the grpc operator
	p.operator = &operator{ibft: p}
	proto.RegisterIbftOperatorServer(srv, p.operator)

	if err := p.createKey(); err != nil {
		return nil, err
	}

	p.logger.Info("validator key", "addr", p.validatorKey.NodeID())

	stdLogger := p.logger.StandardLogger(&hclog.StandardLoggerOptions{})

	p.pbft = pbft.New(
		p.validatorKey,
		&pbftTransport{p},
		pbft.WithLogger(stdLogger),
	)

	// start the transport protocol
	if err := p.setupTransport(); err != nil {
		return nil, err
	}

	return p, nil
}

//TODO
var stateSyncEvent = abi.MustNewEvent(`event Transfer(address token, address to, uint256 amount)`)

func (i *Ibft) setupBridge() error {
	stdLogger := i.logger.StandardLogger(&hclog.StandardLoggerOptions{})

	// create the pool
	tr := &messagePoolTransport{i: i}
	if err := tr.init(); err != nil {
		return err
	}
	i.pool = pool.NewMessagePool(stdLogger, pbft.NodeID(i.validatorKey.String()), tr)

	// NOTE: We need to initialize the pool because it will discard any messages that are not
	// in the validator set and at this point it does not have validator set.
	val := i.getValidators(i.blockchain.Header())
	dd := &dummySet{set: val}
	i.pool.Reset(dd)

	// start the event tracker
	addr, metadata := contracts2.GetRootChain()

	provider, err := jsonrpc.NewClient(addr)
	if err != nil {
		panic(err)
	}
	store, err := boltdbStore.New(i.config.Path + "/deposit.db")
	if err != nil {
		panic(err)
	}

	i.logger.Info("Start tracking events")

	fmt.Println("-- bridge --")
	fmt.Println(metadata.Bridge)

	tt, err := tracker.NewTracker(provider.Eth(),
		tracker.WithBatchSize(2000),
		tracker.WithStore(store),
		tracker.WithFilter(&tracker.FilterConfig{
			Async: true,
			Address: []web3.Address{
				metadata.Bridge,
			},
		}),
	)
	if err != nil {
		panic(err)
	}

	go func() {
		go func() {
			if err := tt.Sync(context.Background()); err != nil {
				fmt.Printf("[ERR]: %v", err)
			}
		}()

		go func() {
			for {
				select {
				case evnt := <-tt.EventCh:
					fmt.Println("-- evnt --")
					fmt.Println(evnt)

					if len(evnt.Removed) != 0 {
						panic("this will not happen anymore after tracker v2")
					}
					for _, log := range evnt.Added {
						// for simplicity, I am going to marshal the whole log
						data, err := log.MarshalJSON()
						if err != nil {
							panic(err)
						}
						i.pool.Add(&pool.Message{
							Data: data,
							From: i.validatorKey.NodeID(),
						}, true)
					}
				case <-tt.DoneCh:
					fmt.Println("historical sync done")
				}
			}
		}()

	}()

	return nil
}

const messagePoolProto = "/pool/0.1"

type messagePoolTransport struct {
	i *Ibft

	topic *network.Topic
}

func (m *messagePoolTransport) init() error {
	// register the gossip protocol
	// V3NOTE: We can piggyback the proto message for ibft though later one we can change it
	topic, err := m.i.network.NewTopic(messagePoolProto, &proto.MessageReq{})
	if err != nil {
		return err
	}
	m.topic = topic

	topic.Subscribe(func(obj interface{}) {
		val := obj.(*proto.MessageReq)

		var msg *pool.Message
		if err := json.Unmarshal(val.Proposal.Value, &msg); err != nil {
			panic(err)
		}
		m.i.pool.Add(msg, false)
	})
	return nil
}

func (m *messagePoolTransport) Gossip(msg *pool.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	req := &proto.MessageReq{
		Proposal: &any.Any{
			Value: data,
		},
	}
	m.topic.Publish(req)
}

type pbftTransport struct {
	i *Ibft
}

func (p *pbftTransport) Gossip(msg *pbft.MessageReq) error {
	protoMsg, err := pbftMsgToProto(msg)
	if err != nil {
		panic(err)
	}
	if err := p.i.transport.Gossip(protoMsg); err != nil {
		panic(err)
	}
	return nil
}

// Start starts the IBFT consensus
func (i *Ibft) Start() error {

	// we need to wait here to start the bridge because it queries the blockchain header and we are not
	// sure up to this point who is the header (only in genesis case though)
	if err := i.setupBridge(); err != nil {
		panic(err)
	}

	// Start the syncer
	i.syncer.Start()

	// Set up the snapshots
	if err := i.setupSnapshot(); err != nil {
		return err
	}

	// Start the actual IBFT protocol
	go i.start()

	return nil
}

type transport interface {
	Gossip(msg *proto.MessageReq) error
}

// Define the IBFT libp2p protocol
var ibftProto = "/ibft/0.1"

type gossipTransport struct {
	topic *network.Topic
}

// Gossip publishes a new message to the topic
func (g *gossipTransport) Gossip(msg *proto.MessageReq) error {
	return g.topic.Publish(msg)
}

// message encoding

func protoMsgToPbft(msg *proto.MessageReq) (*pbft.MessageReq, error) {
	var typ pbft.MsgType
	switch msg.Type {
	case proto.MessageReq_Commit:
		typ = pbft.MessageReq_Commit
	case proto.MessageReq_Prepare:
		typ = pbft.MessageReq_Prepare
	case proto.MessageReq_Preprepare:
		typ = pbft.MessageReq_Preprepare
	case proto.MessageReq_RoundChange:
		typ = pbft.MessageReq_RoundChange
	default:
		panic("bad")
	}

	seal, err := hex.DecodeString(msg.Seal)
	if err != nil {
		return nil, err
	}
	realMsg := &pbft.MessageReq{
		Type: typ,
		From: pbft.NodeID(msg.From),
		Seal: seal,
		View: &pbft.View{
			Sequence: msg.View.Sequence,
			Round:    msg.View.Round,
		},
		Proposal: msg.Proposal.Value,
	}
	return realMsg, nil
}

func pbftMsgToProto(msg *pbft.MessageReq) (*proto.MessageReq, error) {
	var typ proto.MessageReq_Type
	switch msg.Type {
	case pbft.MessageReq_Commit:
		typ = proto.MessageReq_Commit
	case pbft.MessageReq_Prepare:
		typ = proto.MessageReq_Prepare
	case pbft.MessageReq_Preprepare:
		typ = proto.MessageReq_Preprepare
	case pbft.MessageReq_RoundChange:
		typ = proto.MessageReq_RoundChange
	default:
		panic("bad")
	}

	realMsg := &proto.MessageReq{
		Type: typ,
		From: string(msg.From),
		Seal: hex.EncodeToString(msg.Seal),
		View: &proto.View{
			Sequence: msg.View.Sequence,
			Round:    msg.View.Round,
		},
		Proposal: &any.Any{
			Value: msg.Proposal,
		},
	}
	return realMsg, nil
}

// setupTransport sets up the gossip transport protocol
func (i *Ibft) setupTransport() error {
	// Define a new topic
	topic, err := i.network.NewTopic(ibftProto, &proto.MessageReq{})
	if err != nil {
		return err
	}

	// Subscribe to the newly created topic
	err = topic.Subscribe(func(obj interface{}) {
		msg := obj.(*proto.MessageReq)

		if !i.isSealing() {
			// if we are not sealing we do not care about the messages
			// but we need to subscribe to propagate the messages
			return
		}

		/*
			// decode sender
			if err := validateMsg(msg); err != nil {
				i.logger.Error("failed to validate msg", "err", err)

				return
			}
		*/

		if msg.From == string(i.validatorKey.NodeID()) {
			// we are the sender, skip this message since we already
			// relay our own messages internally.
			return
		}

		i.logger.Info("message", "from", msg.From, "typ", msg.Type.String())

		pbftMsg, err := protoMsgToPbft(msg)
		if err != nil {
			panic(err)
		}
		i.pbft.PushMessage(pbftMsg)

		// i.pushMessage(msg)
	})

	if err != nil {
		return err
	}

	i.transport = &gossipTransport{topic: topic}

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

/*
// deprecated
type fsm struct {
	parent *types.Header

	// last proposer
	lastProposer types.Address

	// reference to the current state for validators
	snap *Snapshot

	i *Ibft
}

func (f *fsm) BuildProposal() (*pbft.Proposal, error) {
	block, err := f.i.buildBlock(f.parent, nil) // deprecated
	if err != nil {
		panic(err)
	}

	data := block.MarshalRLP()
	proposal := &pbft.Proposal{
		Time: time.Unix(int64(block.Header.Timestamp), 0),
		Data: data,
	}
	return proposal, nil
}

// Validate validates a raw proposal (used if non-proposer)
func (f *fsm) Validate(proposal []byte) error {
	// it is good right now
	return nil
}

// Insert inserts the sealed proposal
func (f *fsm) Insert(p *pbft.SealedProposal) error {

	//fmt.Println("-- proposal --")
	//fmt.Println(p)
	//fmt.Println(p.Proposal)
	//fmt.Println(p.CommittedSeals)

	block := &types.Block{}
	if err := block.UnmarshalRLP(p.Proposal); err != nil {
		panic(err)
	}
	if err := f.i.insertBlock(block, p.CommittedSeals); err != nil {
		panic(err)
	}
	return nil
}

// Height returns the height for the current round
func (f *fsm) Height() uint64 {
	return f.parent.Number + 1
}

// ValidatorSet returns the validator set for the current round
func (f *fsm) ValidatorSet() pbft.ValidatorSet {

	fmt.Println("-- validator set --")
	fmt.Println(f.snap.Set)

	dd := &dummySet{set: f.snap.Set, lastProposer: f.lastProposer}
	return dd
}

// Hash hashes the proposal bytes
func (f *fsm) Hash(p []byte) []byte {

	// here you return the data that nodes are going to seal for the committed seal
	// in our case, we are going to hash the header (without some fields) and then
	// prefixed with the commit msg

	block := &types.Block{}
	if err := block.UnmarshalRLP(p); err != nil {
		panic(err)
	}
	hash, err := calculateHeaderHash(block.Header)
	if err != nil {
		panic(err)
	}

	fmt.Println("-- hash --")
	fmt.Println(block.Header)
	fmt.Println(hash)

	msg := commitMsg(hash)

	fmt.Println("-- msg --")
	fmt.Println(msg)

	return msg
}

// IsStuck returns whether the pbft is stucked
func (f *fsm) IsStuck(num uint64) (uint64, bool) {
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
}

func (f *fsm) init() error {
	snap, err := f.i.getSnapshot(f.parent.Number)
	if err != nil {
		panic(err)
	}
	f.snap = snap

	var lastProposer types.Address
	if f.parent.Number != 0 {
		lastProposer, err = ecrecoverFromHeader(f.parent)
		if err != nil {
			return err
		}
	}
	f.lastProposer = lastProposer
	return nil
}
*/

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

type fsm2 struct {
	i            *Ibft
	parent       *types.Header
	validators   []types.Address
	lastProposer types.Address
}

func (f *fsm2) init() error {
	f.validators = f.i.getValidators(f.parent)

	var err error

	var lastProposer types.Address
	if f.parent.Number != 0 {
		lastProposer, err = ecrecoverFromHeader(f.parent)
		if err != nil {
			return err
		}
	}
	f.lastProposer = lastProposer
	return nil
}

const epochSize = 10

func (f *fsm2) isEndOfEpoch() bool {
	return f.Height()%10 == 0
}

func (f *fsm2) BuildProposal() (*pbft.Proposal, error) {

	// SEVERAL WAYS TO APPROACH THE PROBLEM OF SENDING CUSTOM TXNS TO THE CLIENT
	// 1. WE EITHER LET THE PROPOSER CREATE THE TXNS AND SEND THEM TO THE VALIDATORS
	// 	  THEN, EACH ONE SHOULD VALIDATE EACH OF THOSE CUSTOM TRANSACTIONS (I.E. IS THIS MESSAGE POOL GOOD?)
	//    (I.E. DOES IT INCLUDES VALIDATORSETCHANGE AT THE END OF EPOCH?)
	// 2. WE ONLY PASS THE BLOCK TRANSACTIONS AND EACH VALIDATOR ON ITS OWN INCLUDES THE TXNS AND BLOCKS
	//    THE ONLY PROBLEM HERE IS THAT WE ARE LOCKING A STRANGE HASH IN PBFT AND WE WILL ONLY KNOW AFTER
	// 	  THE FINAL BLOCK IS INCLUDE IS SOMETHING WENT WRONG. (THIS IS, STATE TXNS ARE NOT PART OF THE PBFT PROTOCOL).
	// 3. A MIX OF BOTH, WE HAVE A GENERIC FUNCTION CALLED GETSTATETXNS() THAT RETURNS ALL THE STATE TRANSACTIONS
	//    DURING THE VALIDATION STAGE, WE CHECK THAT ALL THIS TRANSACTIONS ARE INCLUDED IN THE LOCKED BLOCK.

	block, err := f.i.buildBlock(f.parent, f.validators, func(v *state.Transition) (txns []*types.Transaction) {
		if !f.isEndOfEpoch() {
			return
		}

		// V3NOTE: If we are at the end of the epoch we try to:
		// 1. fit as many state sync as possible from the pool
		// Create a transaction with the item, this will be special state transaction (for now)
		// that does not check the sender.
		for _, msg := range f.i.pool.GetReady() {
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
			transaction := &types.Transaction{
				Input:    input,
				To:       &tokenAddr,
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(0),
			}
			fmt.Printf("---> STATE SYNC CONTRACT: %s %s %d\n", tokenAddr, toAddr, amount)

			// V3NOTE: IMPORTANT FOR THIS THINGS TO USE WRITE() BECAUSE THIS FUNCTION WILL INTERNALLY HANDLE EVERYTHING
			// OF STATESYNC FUNCTIONS. OTHERWISE, YOU MIGHT MISS SOME THINGS THAT ARE PART OF THE CONSENSUS AS WELL.
			if err := v.Write(transaction); err != nil {
				panic(err)
			}
			txns = append(txns, transaction)
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
			transaction := &types.Transaction{
				Input:    input,
				To:       &contracts2.ValidatorContractAddr,
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(0),
			}
			fmt.Println("---> UPDATE VALIDATOR SET <---")

			if err := v.Write(transaction); err != nil {
				panic(err)
			}
			txns = append(txns, transaction)
		}

		return
	})
	if err != nil {
		panic(err)
	}

	data := block.MarshalRLP()
	proposal := &pbft.Proposal{
		Time: time.Unix(int64(block.Header.Timestamp), 0),
		Data: data,
	}
	return proposal, nil
}

func (f *fsm2) Validate(proposal []byte) error {
	return nil
}

func (f *fsm2) Insert(p *pbft.SealedProposal) error {

	block := &types.Block{}
	if err := block.UnmarshalRLP(p.Proposal); err != nil {
		panic(err)
	}
	if err := f.i.insertBlock(block, p.CommittedSeals); err != nil {
		panic(err)
	}
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
	block := &types.Block{}
	if err := block.UnmarshalRLP(p); err != nil {
		panic(err)
	}
	hash, err := calculateHeaderHash(block.Header)
	if err != nil {
		panic(err)
	}

	//fmt.Println("-- hash --")
	//fmt.Println(block.Header)
	//fmt.Println(hash)

	msg := commitMsg(hash)

	//fmt.Println("-- msg --")
	//fmt.Println(msg)

	return msg
}

func (f *fsm2) IsStuck(num uint64) (uint64, bool) {
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
}

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
	//fmt.Println("X")
	// try to sync as much as possible
	i.runSyncState()
	//we need to remove this since it uses the old snapshot
	//fmt.Println("Y")
	for {
		parent := i.blockchain.Header()

		// read the current validators from the contract
		fsm := &fsm2{
			i:      i,
			parent: parent,
		}

		// initialize the wrapper
		if err := fsm.init(); err != nil {
			panic(err)
		}
		if err := i.pbft.SetBackend(fsm); err != nil {
			panic(err)
		}

		// start the pbft sequence
		i.pbft.Run(context.Background())

		switch i.pbft.GetState() {
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

/*
// runCycle represents the IBFT state machine loop
func (i *Ibft) runCycle() {
	// Log to the console
	if i.state.view != nil {
		i.logger.Debug("cycle", "state", i.getState(), "sequence", i.state.view.Sequence, "round", i.state.view.Round)
	}

	// Based on the current state, execute the corresponding section
	switch i.getState() {
	case AcceptState:
		i.runAcceptState()

	case ValidateState:
		i.runValidateState()

	case RoundChangeState:
		i.runRoundChangeState()

	case SyncState:
		i.runSyncState()
	}
}
*/

/*
// isValidSnapshot checks if the current node is in the validator set for the latest snapshot
func (i *Ibft) isValidSnapshot() bool {
	if !i.isSealing() {
		return false
	}

	// check if we are a validator and enabled
	header := i.blockchain.Header()
	snap, err := i.getSnapshot(header.Number)
	if err != nil {
		return false
	}

	if snap.Set.Includes() {
		// Do not need to do this since it will be done later
		i.state.view = &proto.View{
			Sequence: header.Number + 1,
			Round:    0,
		}
		return true
	}
	return false
}
*/

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

		/*
			snap, err := i.getSnapshot(header.Number)
			if err != nil {
				return false
			}
		*/

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
func (i *Ibft) buildBlock( /*snap *Snapshot, */ parent *types.Header, validators []types.Address, handler func(t *state.Transition) []*types.Transaction) (*types.Block, error) {
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     parent.Number + 1,
		Miner:      types.Address{},
		Nonce:      types.Nonce{},
		MixHash:    IstanbulDigest,
		Difficulty: parent.Number + 1,   // we need to do this because blockchain needs difficulty to organize blocks and forks
		StateRoot:  types.EmptyRootHash, // this avoids needing state for now
		Sha3Uncles: types.EmptyUncleHash,
		GasLimit:   parent.GasLimit, // Inherit from parent for now, will need to adjust dynamically later.
	}

	//fmt.Println("--")
	//fmt.Println(i)
	//fmt.Println(i.blockchain)
	//fmt.Println(header)

	// calculate gas limit based on parent header
	gasLimit, err := i.blockchain.CalculateGasLimit(header.Number)
	if err != nil {
		return nil, err
	}
	header.GasLimit = gasLimit

	/*
		// try to pick a candidate
		if candidate := i.operator.getNextCandidate(snap); candidate != nil {
			header.Miner = types.StringToAddress(candidate.Address)
			if candidate.Auth {
				header.Nonce = nonceAuthVote
			} else {
				header.Nonce = nonceDropVote
			}
		}
	*/

	// set the timestamp
	parentTime := time.Unix(int64(parent.Timestamp), 0)
	headerTime := parentTime.Add(defaultBlockPeriod)

	if headerTime.Before(time.Now()) {
		headerTime = time.Now()
	}
	header.Timestamp = uint64(headerTime.Unix())

	// we need to include in the extra field the current set of validators
	putIbftExtraValidators(header, validators)

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
	stateTxns := handler(transition)
	txns = append(txns, stateTxns...)

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

	// write the seal of the block after all the fields are completed
	header, err = writeSeal(i.validatorKey.priv, block.Header)
	if err != nil {
		return nil, err
	}
	block.Header = header

	// compute the hash, this is only a provisional hash since the final one
	// is sealed after all the committed seals
	block.Header.ComputeHash()

	i.logger.Info("build block", "number", header.Number, "txns", len(txns))
	return block, nil
}

/*
// runAcceptState runs the Accept state loop
//
// The Accept state always checks the snapshot, and the validator set. If the current node is not in the validators set,
// it moves back to the Sync state. On the other hand, if the node is a validator, it calculates the proposer.
// If it turns out that the current node is the proposer, it builds a block, and sends preprepare and then prepare messages.
func (i *Ibft) runAcceptState() { // start new round
	logger := i.logger.Named("acceptState")
	logger.Info("Accept state", "sequence", i.state.view.Sequence)

	// This is the state in which we either propose a block or wait for the pre-prepare message
	parent := i.blockchain.Header()
	number := parent.Number + 1
	if number != i.state.view.Sequence {
		i.logger.Error("sequence not correct", "parent", parent.Number, "sequence", i.state.view.Sequence)
		i.setState(SyncState)
		return
	}
	snap, err := i.getSnapshot(parent.Number)
	if err != nil {
		i.logger.Error("cannot find snapshot", "num", parent.Number)
		i.setState(SyncState)
		return
	}

	if !snap.Set.Includes(i.validatorKeyAddr) {
		// we are not a validator anymore, move back to sync state
		i.logger.Info("we are not a validator anymore")
		i.setState(SyncState)
		return
	}

	i.logger.Info("current snapshot", "validators", len(snap.Set), "votes", len(snap.Votes))

	i.state.validators = snap.Set

	// reset round messages
	i.state.resetRoundMsgs()

	// select the proposer of the block
	var lastProposer types.Address
	if parent.Number != 0 {
		lastProposer, _ = ecrecoverFromHeader(parent)
	}

	i.state.CalcProposer(lastProposer)

	if i.state.proposer == i.validatorKeyAddr {
		logger.Info("we are the proposer", "block", number)

		if !i.state.locked {
			// since the state is not locked, we need to build a new block
			i.state.block, err = i.buildBlock(snap, parent)
			if err != nil {
				i.logger.Error("failed to build block", "err", err)
				i.setState(RoundChangeState)
				return
			}

			// calculate how much time do we have to wait to mine the block
			delay := time.Until(time.Unix(int64(i.state.block.Header.Timestamp), 0))

			select {
			case <-time.After(delay):
			case <-i.closeCh:
				return
			}
		}

		// send the preprepare message as an RLP encoded block
		i.sendPreprepareMsg()

		// send the prepare message since we are ready to move the state
		i.sendPrepareMsg()

		// move to validation state for new prepare messages
		i.setState(ValidateState)
		return
	}

	i.logger.Info("proposer calculated", "proposer", i.state.proposer, "block", number)

	// we are NOT a proposer for the block. Then, we have to wait
	// for a pre-prepare message from the proposer

	timeout := i.randomTimeout()
	for i.getState() == AcceptState {
		msg, ok := i.getNextMessage(timeout)
		if !ok {
			return
		}
		if msg == nil {
			i.setState(RoundChangeState)
			continue
		}

		if msg.From != i.state.proposer.String() {
			i.logger.Error("msg received from wrong proposer")
			continue
		}

		// retrieve the block proposal
		block := &types.Block{}
		if err := block.UnmarshalRLP(msg.Proposal.Value); err != nil {
			i.logger.Error("failed to unmarshal block", "err", err)
			i.setState(RoundChangeState)
			return
		}
		if i.state.locked {
			// the state is locked, we need to receive the same block
			if block.Hash() == i.state.block.Hash() {
				// fast-track and send a commit message and wait for validations
				i.sendCommitMsg()
				i.setState(ValidateState)
			} else {
				i.handleStateErr(errIncorrectBlockLocked)
			}
		} else {
			// since its a new block, we have to verify it first
			if err := i.verifyHeaderImpl(snap, parent, block.Header); err != nil {
				i.logger.Error("block verification failed", "err", err)
				i.handleStateErr(errBlockVerificationFailed)
			} else {
				i.state.block = block

				// send prepare message and wait for validations
				i.sendPrepareMsg()
				i.setState(ValidateState)
			}
		}
	}
}

// runValidateState implements the Validate state loop.
//
// The Validate state is rather simple - all nodes do in this state is read messages and add them to their local snapshot state
func (i *Ibft) runValidateState() {
	hasCommitted := false
	sendCommit := func() {
		// at this point either we have enough prepare messages
		// or commit messages so we can lock the block
		i.state.lock()

		if !hasCommitted {
			// send the commit message
			i.sendCommitMsg()
			hasCommitted = true
		}
	}

	timeout := i.randomTimeout()
	for i.getState() == ValidateState {
		msg, ok := i.getNextMessage(timeout)
		if !ok {
			// closing
			return
		}
		if msg == nil {
			i.setState(RoundChangeState)
			continue
		}

		switch msg.Type {
		case proto.MessageReq_Prepare:
			i.state.addPrepared(msg)

		case proto.MessageReq_Commit:
			i.state.addCommitted(msg)

		default:
			panic(fmt.Sprintf("BUG: %s", reflect.TypeOf(msg.Type)))
		}

		if i.state.numPrepared() > i.state.NumValid() {
			// we have received enough pre-prepare messages
			sendCommit()
		}

		if i.state.numCommitted() > i.state.NumValid() {
			// we have received enough commit messages
			sendCommit()

			// try to commit the block (TODO: just to get out of the loop)
			i.setState(CommitState)
		}
	}

	if i.getState() == CommitState {
		// at this point either if it works or not we need to unlock
		block := i.state.block
		i.state.unlock()

		if err := i.insertBlock(block); err != nil {
			// start a new round with the state unlocked since we need to
			// be able to propose/validate a different block
			i.logger.Error("failed to insert block", "err", err)
			i.handleStateErr(errFailedToInsertBlock)
		} else {
			// move ahead to the next block
			i.setState(AcceptState)
		}
	}
}
*/

func (i *Ibft) insertBlock(block *types.Block, committedSeals [][]byte) error {
	/*
		committedSeals := [][]byte{}
		for _, commit := range i.state.committed {
			// no need to check the format of seal here because writeCommittedSeals will check
			committedSeals = append(committedSeals, hex.MustDecodeHex(commit.Seal))
		}
	*/

	header, err := writeCommittedSeals(block.Header, committedSeals)
	if err != nil {
		return err
	}

	// we need to recompute the hash since we have change extra-data
	block.Header = header
	block.Header.ComputeHash()

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

	/*
		// increase the sequence number and reset the round if any
		i.state.view = &proto.View{
			Sequence: header.Number + 1,
			Round:    0,
		}
	*/

	// broadcast the new block
	i.syncer.Broadcast(block)

	// after the block has been written we reset the txpool so that
	// the old transactions are removed
	i.txpool.ResetWithHeader(block.Header)

	return nil
}

/*
var (
	errIncorrectBlockLocked    = fmt.Errorf("block locked is incorrect")
	errBlockVerificationFailed = fmt.Errorf("block verification failed")
	errFailedToInsertBlock     = fmt.Errorf("failed to insert block")
)

func (i *Ibft) handleStateErr(err error) {
	i.state.err = err
	i.setState(RoundChangeState)
}

func (i *Ibft) runRoundChangeState() {
	sendRoundChange := func(round uint64) {
		i.logger.Debug("local round change", "round", round)
		// set the new round
		i.state.view.Round = round
		// clean the round
		i.state.cleanRound(round)
		// send the round change message
		i.sendRoundChange()
	}
	sendNextRoundChange := func() {
		sendRoundChange(i.state.view.Round + 1)
	}

	checkTimeout := func() {
		// check if there is any peer that is really advanced and we might need to sync with it first
		if i.syncer != nil {
			bestPeer := i.syncer.BestPeer()
			if bestPeer != nil {
				lastProposal := i.blockchain.Header()
				if bestPeer.Number() > lastProposal.Number {
					i.logger.Debug("it has found a better peer to connect", "local", lastProposal.Number, "remote", bestPeer.Number())
					// we need to catch up with the last sequence
					i.setState(SyncState)
					return
				}
			}
		}

		// otherwise, it seems that we are in sync
		// and we should start a new round
		sendNextRoundChange()
	}

	// if the round was triggered due to an error, we send our own
	// next round change
	if err := i.state.getErr(); err != nil {
		i.logger.Debug("round change handle err", "err", err)
		sendNextRoundChange()
	} else {
		// otherwise, it is due to a timeout in any stage
		// First, we try to sync up with any max round already available
		if maxRound, ok := i.state.maxRound(); ok {
			i.logger.Debug("round change set max round", "round", maxRound)
			sendRoundChange(maxRound)
		} else {
			// otherwise, do your best to sync up
			checkTimeout()
		}
	}

	// create a timer for the round change
	timeout := i.randomTimeout()
	for i.getState() == RoundChangeState {
		msg, ok := i.getNextMessage(timeout)
		if !ok {
			// closing
			return
		}
		if msg == nil {
			i.logger.Debug("round change timeout")
			checkTimeout()
			//update the timeout duration
			timeout = i.randomTimeout()
			continue
		}

		// we only expect RoundChange messages right now
		num := i.state.AddRoundMessage(msg)

		if num == i.state.NumValid() {
			// start a new round inmediatly
			i.state.view.Round = msg.View.Round
			i.setState(AcceptState)
		} else if num == i.state.validators.MaxFaultyNodes()+1 {
			// weak certificate, try to catch up if our round number is smaller
			if i.state.view.Round < msg.View.Round {
				// update timer
				timeout = i.randomTimeout()
				sendRoundChange(msg.View.Round)
			}
		}
	}
}
*/

// --- com wrappers ---

/*
func (i *Ibft) sendRoundChange() {
	i.gossip(proto.MessageReq_RoundChange)
}

func (i *Ibft) sendPreprepareMsg() {
	i.gossip(proto.MessageReq_Preprepare)
}

func (i *Ibft) sendPrepareMsg() {
	i.gossip(proto.MessageReq_Prepare)
}

func (i *Ibft) sendCommitMsg() {
	i.gossip(proto.MessageReq_Commit)
}

func (i *Ibft) gossip(typ proto.MessageReq_Type) {
	msg := &proto.MessageReq{
		Type: typ,
	}

	// add View
	msg.View = i.state.view.Copy()

	// if we are sending a preprepare message we need to include the proposed block
	if msg.Type == proto.MessageReq_Preprepare {
		msg.Proposal = &any.Any{
			Value: i.state.block.MarshalRLP(),
		}
	}

	// if the message is commit, we need to add the committed seal
	if msg.Type == proto.MessageReq_Commit {
		seal, err := writeCommittedSeal(i.validatorKey, i.state.block.Header)
		if err != nil {
			i.logger.Error("failed to commit seal", "err", err)
			return
		}
		msg.Seal = hex.EncodeToHex(seal)
	}

	if msg.Type != proto.MessageReq_Preprepare {
		// send a copy to ourselves so that we can process this message as well
		msg2 := msg.Copy()
		msg2.From = i.validatorKeyAddr.String()
		i.pushMessage(msg2)
	}
	if err := signMsg(i.validatorKey, msg); err != nil {
		i.logger.Error("failed to sign message", "err", err)
		return
	}
	if err := i.transport.Gossip(msg); err != nil {
		i.logger.Error("failed to gossip", "err", err)
	}
}

// getState returns the current IBFT state
func (i *Ibft) getState() IbftState {
	return i.state.getState()
}

// isState checks if the node is in the passed in state
func (i *Ibft) isState(s IbftState) bool {
	return i.state.getState() == s
}

// setState sets the IBFT state
func (i *Ibft) setState(s IbftState) {
	i.logger.Debug("state change", "new", s)
	i.state.setState(s)
}

// forceTimeout sets the forceTimeoutCh flag to true
func (i *Ibft) forceTimeout() {
	i.forceTimeoutCh = true
}

// randomTimeout calculates the timeout duration depending on the current round
func (i *Ibft) randomTimeout() time.Duration {
	timeout := time.Duration(10000) * time.Millisecond
	round := i.state.view.Round
	if round > 0 {
		timeout += time.Duration(math.Pow(2, float64(round))) * time.Second
	}
	return timeout
}
*/

// isSealing checks if the current node is sealing blocks
func (i *Ibft) isSealing() bool {
	return i.sealing
}

// verifyHeaderImpl implements the actual header verification logic
func (i *Ibft) verifyHeaderImpl(parent, header *types.Header) error {
	// ensure the extra data is correctly formatted
	if _, err := getIbftExtra(header); err != nil {
		return err
	}

	// Because you must specify either AUTH or DROP vote, it is confusing how to have a block without any votes.
	// 		This is achieved by specifying the miner field to zeroes,
	// 		because then the value in the Nonce will not be taken into consideration.
	if header.Nonce != nonceDropVote && header.Nonce != nonceAuthVote {
		return fmt.Errorf("invalid nonce")
	}

	if header.MixHash != IstanbulDigest {
		return fmt.Errorf("invalid mixhash")
	}

	if header.Sha3Uncles != types.EmptyUncleHash {
		return fmt.Errorf("invalid sha3 uncles")
	}

	// difficulty has to match number
	if header.Difficulty != header.Number {
		return fmt.Errorf("wrong difficulty")
	}

	/*
		// V3NOTE: I KNOW, WE HAVE TO ENABLE THIS
		// verify the sealer
		if err := verifySigner(snap, header); err != nil {
			return err
		}
	*/

	return nil
}

// VerifyHeader wrapper for verifying headers
func (i *Ibft) VerifyHeader(parent, header *types.Header) error {
	/*
		snap, err := i.getSnapshot(parent.Number)
		if err != nil {
			return err
		}
	*/

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

	/*
		// process the new block in order to update the snapshot
		if err := i.processHeaders([]*types.Header{header}); err != nil {
			return err
		}
	*/

	return nil
}

// GetBlockCreator retrieves the block signer from the extra data field
func (i *Ibft) GetBlockCreator(header *types.Header) (types.Address, error) {
	return ecrecoverFromHeader(header)
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

/*
// getNextMessage reads a new message from the message queue
func (i *Ibft) getNextMessage(timeout time.Duration) (*proto.MessageReq, bool) {
	timeoutCh := time.After(timeout)
	for {
		msg := i.msgQueue.readMessage(i.getState(), i.state.view)
		if msg != nil {
			return msg.obj, true
		}

		if i.forceTimeoutCh {
			i.forceTimeoutCh = false
			return nil, true
		}

		// wait until there is a new message or
		// someone closes the stopCh (i.e. timeout for round change)
		select {
		case <-timeoutCh:
			return nil, true
		case <-i.closeCh:
			return nil, false
		case <-i.updateCh:
		}
	}
}

// pushMessage pushes a new message to the message queue
func (i *Ibft) pushMessage(msg *proto.MessageReq) {
	task := &msgTask{
		view: msg.View,
		msg:  protoTypeToMsg(msg.Type),
		obj:  msg,
	}
	i.msgQueue.pushMessage(task)

	select {
	case i.updateCh <- struct{}{}:
	default:
	}
}
*/
