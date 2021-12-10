package polybft

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/0xPolygon/polygon-sdk/types"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-sdk/consensus/polybft/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	"github.com/umbracle/go-web3/jsonrpc"
	"github.com/umbracle/go-web3/tracker"
	boltdbStore "github.com/umbracle/go-web3/tracker/store/boltdb"
)

type PolyBFT struct {
	logger *log.Logger

	config *Config

	// pbft is the PBFT consensus engine
	pbft *pbft.Pbft

	// pool is the message pool for the decentralized bridge
	pool *MessagePool

	Executor Executor

	Backend Backend
	network Transport

	key web3.Key

	path string
}

type Executor interface {
	Call(parent *types.Header, to types.Address, data []byte) ([]byte, error)
}

type Config struct {
	// JsonRPCEndpoint is the JSONRPC endpoint of the rootchain server
	JSONRPCEndpoint string

	// Address of the validator contract in sidechain
	ValidatorContractAddr types.Address

	// Address of the bridge contract in rootchain
	BridgeAddr types.Address
}

func NewPolyBFT(logger *log.Logger, config *Config, path string, key web3.Key, Executor Executor, Backend Backend, network Transport) (*PolyBFT, error) {
	p := &PolyBFT{
		logger:   logger,
		config:   config,
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
		&wrapKey{key},
		&pbftTransport{topic},
		pbft.WithLogger(logger),
	)

	return p, nil
}

type wrapKey struct {
	k web3.Key
}

func (w *wrapKey) NodeID() pbft.NodeID {
	return pbft.NodeID(w.k.Address().String())
}

func (w *wrapKey) Sign(b []byte) ([]byte, error) {
	return w.k.Sign(web3.Keccak256(b))
}

func (p *PolyBFT) NodeID() pbft.NodeID {
	return pbft.NodeID(p.key.Address().String())
}

func (p *PolyBFT) VerifyHeader(header *types.Header) error {
	// ensure the extra data is correctly formatted
	if _, err := GetIbftExtra(header); err != nil {
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
	PutIbftExtraValidators(header, validators)

	// write the seal of the block after all the fields are completed
	header, err := writeSeal(p.key, block.Header)
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

	if dd.Includes(p.NodeID()) {
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
func (p *PolyBFT) setupTransport() (Topic, error) {
	// Define a new topic
	topic, err := p.network.NewTopic(ibftProto, &proto.MessageReq{})
	if err != nil {
		return nil, err
	}

	// Subscribe to the newly created topic
	err = topic.Subscribe(func(obj interface{}) {
		msg := obj.(*proto.MessageReq)

		if msg.From == string(p.NodeID()) {
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
	topic Topic
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

	returnValue, err := p.Executor.Call(parent, p.config.ValidatorContractAddr, xx.Methods["getValidators"].ID())
	if err != nil {
		panic(err)
	}

	raw, err := xx.Methods["getValidators"].Decode(returnValue)
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
	p.pool = NewMessagePool(p.logger, p.NodeID(), tr)

	// NOTE: We need to initialize the pool because it will discard any messages that are not
	// in the validator set and at this point it does not have validator set.
	val := p.getValidators(p.Backend.Header())
	dd := &dummySet{set: val}
	p.pool.Reset(dd)

	// start the event tracker
	// addr, metadata := contracts2.GetRootChain()

	provider, err := jsonrpc.NewClient(p.config.JSONRPCEndpoint)
	if err != nil {
		panic(err)
	}
	store, err := boltdbStore.New(p.path + "/deposit.db")
	if err != nil {
		panic(err)
	}

	p.logger.Printf("[INFO] Start tracking events: bridge=%s", p.config.BridgeAddr)

	tt, err := tracker.NewTracker(provider.Eth(),
		tracker.WithBatchSize(2000),
		tracker.WithStore(store),
		tracker.WithFilter(&tracker.FilterConfig{
			Async: true,
			Address: []web3.Address{
				web3.Address(p.config.BridgeAddr),
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
							From: p.NodeID(),
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
