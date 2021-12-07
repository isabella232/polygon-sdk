package polybft

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"

	"github.com/0xPolygon/polygon-sdk/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-sdk/contracts2"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/types"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	"github.com/umbracle/go-web3/jsonrpc"
	"github.com/umbracle/go-web3/tracker"
	boltdbStore "github.com/umbracle/go-web3/tracker/store/boltdb"
)

type PolyBFT struct {
	logger *log.Logger

	// pbft is the PBFT consensus engine
	pbft *pbft.Pbft

	// message pool
	pool *MessagePool

	Executor *state.Executor

	Backend Backend
	network *network.Server

	key *key

	path string
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

func NewKey(priv *ecdsa.PrivateKey) *key {
	k := &key{
		priv: priv,
		addr: crypto.PubKeyToAddress(&priv.PublicKey),
	}
	return k
}

func NewPolyBFT(logger *log.Logger, path string, key *key, Executor *state.Executor, Backend Backend, network *network.Server) (*PolyBFT, error) {
	p := &PolyBFT{
		logger:   logger,
		Executor: Executor,
		Backend:  Backend,
		network:  network,
		key:      key,
		path:     path,
	}

	// start transport for pbft
	topic, err := p.setupTransport()
	if err != nil {
		panic(err)
	}

	p.pbft = pbft.New(
		key,
		&pbftTransport{topic},
		pbft.WithLogger(logger),
	)

	return p, nil
}

func (p *PolyBFT) VerifyHeader(header *types.Header) error {
	// ensure the extra data is correctly formatted
	if _, err := getIbftExtra(header); err != nil {
		return err
	}

	if header.MixHash != IstanbulDigest {
		return fmt.Errorf("invalid mixhash")
	}

	return nil
}

func (p *PolyBFT) BlockCreator(header *types.Header) (types.Address, error) {
	return ecrecoverFromHeader(header)
}

func (p *PolyBFT) Finish(block *types.Block, validators []types.Address) error {
	header := block.Header
	header.MixHash = IstanbulDigest

	// we need to include in the extra field the current set of validators
	putIbftExtraValidators(header, validators)

	// write the seal of the block after all the fields are completed
	header, err := writeSeal(p.key.priv, block.Header)
	if err != nil {
		panic(err)
	}
	block.Header = header

	return nil
}

func (p *PolyBFT) IsValidator() bool {
	// check if we are a validator and enabled
	header := p.Backend.Header()

	val := p.getValidators(header)
	dd := &dummySet{set: val, lastProposer: types.Address{}}

	/*
		snap, err := i.getSnapshot(header.Number)
		if err != nil {
			return false
		}
	*/

	if dd.Includes(pbft.NodeID(p.key.addr.String())) {
		// Do not need to do this since it will be done later
		return true
	}
	return false
}

func (p *PolyBFT) Setup() {
	if err := p.setupBridge(); err != nil {
		panic(err)
	}
}

func (p *PolyBFT) GetState() pbft.PbftState {
	return p.pbft.GetState()
}

func (p *PolyBFT) Run(parent *types.Header) {
	fsm := &fsm2{
		parent: parent,
		b:      p.Backend,
		p:      p,
	}

	// initialize the wrapper
	if err := fsm.init(); err != nil {
		panic(err)
	}
	if err := p.pbft.SetBackend(fsm); err != nil {
		panic(err)
	}

	// start the pbft sequence
	p.pbft.Run(context.Background())
}

var ibftProto = "/ibft/0.1"

// setupTransport sets up the gossip transport protocol
func (p *PolyBFT) setupTransport() (*network.Topic, error) {
	// Define a new topic
	topic, err := p.network.NewTopic(ibftProto, &proto.MessageReq{})
	if err != nil {
		return nil, err
	}

	// Subscribe to the newly created topic
	err = topic.Subscribe(func(obj interface{}) {
		msg := obj.(*proto.MessageReq)

		if msg.From == string(p.key.NodeID()) {
			// we are the sender, skip this message since we already
			// relay our own messages internally.
			return
		}

		// i.logger.Info("message", "from", msg.From, "typ", msg.Type.String())

		pbftMsg, err := protoMsgToPbft(msg)
		if err != nil {
			panic(err)
		}
		p.pbft.PushMessage(pbftMsg)

		// i.pushMessage(msg)
	})

	if err != nil {
		return nil, err
	}

	return topic, nil
}

type pbftTransport struct {
	topic *network.Topic
}

func (p *pbftTransport) Gossip(msg *pbft.MessageReq) error {
	protoMsg, err := pbftMsgToProto(msg)
	if err != nil {
		panic(err)
	}

	if err := p.topic.Publish(protoMsg); err != nil {
		return err
	}
	return nil
}

func (p *PolyBFT) getValidators(parent *types.Header) []types.Address {
	// get a state reference for this to make calls and txns!
	// to call we need to use the parent reference

	xx, err := abi.NewABIFromList([]string{
		"function getValidators() returns (address[])",
	})
	if err != nil {
		panic(err)
	}

	tt, err := p.Executor.BeginTxn(parent.StateRoot, parent, types.Address{})
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

// ... bridge ...

func (p *PolyBFT) setupBridge() error {
	// create the pool
	tr := &messagePoolTransport{i: p}
	if err := tr.init(); err != nil {
		return err
	}
	p.pool = NewMessagePool(p.logger, pbft.NodeID(p.key.String()), tr)

	// NOTE: We need to initialize the pool because it will discard any messages that are not
	// in the validator set and at this point it does not have validator set.
	val := p.getValidators(p.Backend.Header())
	dd := &dummySet{set: val}
	p.pool.Reset(dd)

	// start the event tracker
	addr, metadata := contracts2.GetRootChain()

	provider, err := jsonrpc.NewClient(addr)
	if err != nil {
		panic(err)
	}
	store, err := boltdbStore.New(p.path + "/deposit.db")
	if err != nil {
		panic(err)
	}

	p.logger.Print("[INFO] Start tracking events")

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
						p.pool.Add(&Message{
							Data: data,
							From: p.key.NodeID(),
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

// ...

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
