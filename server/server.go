package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/consensus/ibft"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/keccak"
	"github.com/0xPolygon/polygon-sdk/jsonrpc"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	"github.com/0xPolygon/polygon-sdk/smart-contract/bindings"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/state/runtime"
	"github.com/0xPolygon/polygon-sdk/txpool"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3/abi"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	itrie "github.com/0xPolygon/polygon-sdk/state/immutable-trie"
	"github.com/0xPolygon/polygon-sdk/state/runtime/evm"
	"github.com/0xPolygon/polygon-sdk/state/runtime/precompiled"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/consensus"
)

// Minimal is the central manager of the blockchain client
type Server struct {
	logger       hclog.Logger
	config       *Config
	state        state.State
	stateStorage itrie.Storage

	consensus consensus.Consensus

	// blockchain stack
	blockchain *blockchain.Blockchain
	chain      *chain.Chain

	// state executor
	executor *state.Executor

	// jsonrpc stack
	jsonrpcServer *jsonrpc.JSONRPC

	// system grpc server
	grpcServer *grpc.Server

	// libp2p network
	network *network.Server

	// transaction pool
	txpool *txpool.TxPool
}

var dirPaths = []string{
	"blockchain",
	"consensus",
	"keystore",
	"trie",
	"libp2p",
}

// NewServer creates a new Minimal server, using the passed in configuration
func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	m := &Server{
		logger:     logger,
		config:     config,
		chain:      config.Chain,
		grpcServer: grpc.NewServer(),
	}

	m.logger.Info("Data dir", "path", config.DataDir)

	// Generate all the paths in the dataDir
	if err := SetupDataDir(config.DataDir, dirPaths); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %v", err)
	}

	// start libp2p
	{
		netConfig := config.Network
		netConfig.Chain = m.config.Chain
		netConfig.DataDir = filepath.Join(m.config.DataDir, "libp2p")

		network, err := network.NewServer(logger, netConfig)
		if err != nil {
			return nil, err
		}
		m.network = network
	}

	// start blockchain object
	stateStorage, err := itrie.NewLevelDBStorage(filepath.Join(m.config.DataDir, "trie"), logger)
	if err != nil {
		return nil, err
	}
	m.stateStorage = stateStorage

	st := itrie.NewState(stateStorage)
	m.state = st

	m.executor = state.NewExecutor(config.Chain.Params, st, logger)
	m.executor.SetRuntime(precompiled.NewPrecompiled())
	m.executor.SetRuntime(evm.NewEVM())

	writeGenesis := func() types.Hash {
		// V3NOTE: We are going tocreate the validator contract as part of genesis because
		// this way we can start not just the smart contract but also the internal data.

		initialValidators, err := readValidatorsByRegexp("test-chain-")
		if err != nil {
			panic(err)
		}

		// This works but the abi from the abigen dont, figure it out. I think it has to be with the ... ternary ops
		xx := abi.MustNewType("tuple(address[] a)")
		methodInput, err := xx.Encode(map[string]interface{}{
			"a": initialValidators,
		})
		if err != nil {
			panic(err)
		}

		// Now we need to send an intiail transaction to set the data in.
		txn, err := m.executor.BeginTxnGenesis()
		if err != nil {
			panic(err)
		}

		input := []byte{}
		input = append(input, bindings.ValidatorBin()...)
		input = append(input, methodInput...)

		// make contract deployment for the validator node
		transaction := &types.Transaction{
			Input:    input,
			To:       nil,
			From:     types.Address{0x1},
			Value:    big.NewInt(0),
			GasPrice: big.NewInt(0),
		}
		address := crypto.CreateAddress(types.Address{0x1}, 0)

		fmt.Println("--- DEPLOY VALIDATOR SOURCE CONTRACT ---")
		fmt.Println(txn.ApplyInt(1000000, transaction))
		// this address is determinsitic, that is why other layers (i.e. ibft) aleady know it.
		fmt.Println(address)

		{
			// DO THE SAME TO DEPLOY THE ERC20 TOKEN, AGAIN DO IT IN A DETERMINISTIC ADDRESS

			// make contract deployment for the validator node
			transaction := &types.Transaction{
				Input:    bindings.MintERC20Bin(),
				To:       nil,
				From:     types.Address{0x2}, // use another address to deploy this
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(0),
			}
			address := crypto.CreateAddress(types.Address{0x2}, 0)

			fmt.Println("--- DEPLOY DESTINY TOKEN CONTRACT ---")
			fmt.Println(txn.ApplyInt(100000000000000, transaction))
			fmt.Println(address)
		}

		_, root := txn.Commit()
		return root

		//_, root := txn.Commit(false)
		//return types.BytesToHash(root)
	}

	genesisRoot := writeGenesis()

	fmt.Println("-- genesis root --")
	fmt.Println(genesisRoot)

	config.Chain.Genesis.StateRoot = genesisRoot

	// blockchain object
	m.blockchain, err = blockchain.NewBlockchain(logger, m.config.DataDir, config.Chain, nil, m.executor)
	if err != nil {
		return nil, err
	}

	m.executor.GetHash = m.blockchain.GetHashHelper

	{
		hub := &txpoolHub{
			state:      m.state,
			Blockchain: m.blockchain,
		}
		// start transaction pool
		m.txpool, err = txpool.NewTxPool(logger, m.config.Seal, m.config.Locals, m.config.NoLocals, m.config.PriceLimit, m.config.MaxSlots, m.chain.Params.Forks.At(0), hub, m.grpcServer, m.network)
		if err != nil {
			return nil, err
		}

		// use the eip155 signer
		signer := crypto.NewEIP155Signer(uint64(m.config.Chain.Params.ChainID))
		m.txpool.AddSigner(signer)
	}

	{
		// Setup consensus
		if err := m.setupConsensus(); err != nil {
			return nil, err
		}
		m.blockchain.SetConsensus(m.consensus)
	}

	// after consensus is done, we can mine the genesis block in blockchain
	// This is done because consensus might use a custom Hash function so we need
	// to wait for consensus because we do any block hashing like genesis
	if err := m.blockchain.ComputeGenesis(); err != nil {
		return nil, err
	}

	// setup grpc server
	if err := m.setupGRPC(); err != nil {
		return nil, err
	}

	// setup jsonrpc
	if err := m.setupJSONRPC(); err != nil {
		return nil, err
	}

	if err := m.consensus.Start(); err != nil {
		return nil, err
	}

	return m, nil
}

type txpoolHub struct {
	state state.State
	*blockchain.Blockchain
}

func (t *txpoolHub) GetNonce(root types.Hash, addr types.Address) uint64 {
	snap, err := t.state.NewSnapshotAt(root)
	if err != nil {
		return 0
	}
	result, ok := snap.Get(keccak.Keccak256(nil, addr.Bytes()))
	if !ok {
		return 0
	}
	var account state.Account
	if err := account.UnmarshalRlp(result); err != nil {
		return 0
	}
	return account.Nonce
}

func (t *txpoolHub) GetBalance(root types.Hash, addr types.Address) (*big.Int, error) {
	snap, err := t.state.NewSnapshotAt(root)
	if err != nil {
		return nil, fmt.Errorf("unable to get snapshot for root, %v", err)
	}

	result, ok := snap.Get(keccak.Keccak256(nil, addr.Bytes()))
	if !ok {
		return big.NewInt(0), nil
	}

	var account state.Account
	if err = account.UnmarshalRlp(result); err != nil {
		return nil, fmt.Errorf("unable to unmarshal account from snapshot, %v", err)
	}

	return account.Balance, nil
}

// setupConsensus sets up the consensus mechanism
func (s *Server) setupConsensus() error {
	engineName := s.config.Chain.Params.GetEngine()
	engine, ok := consensusBackends[engineName]
	if !ok {
		return fmt.Errorf("consensus engine '%s' not found", engineName)
	}

	engineConfig, ok := s.config.Chain.Params.Engine[engineName].(map[string]interface{})
	if !ok {
		engineConfig = map[string]interface{}{}
	}
	config := &consensus.Config{
		Params: s.config.Chain.Params,
		Config: engineConfig,
		Path:   filepath.Join(s.config.DataDir, "consensus"),
	}
	consensus, err := engine(context.Background(), s.config.Seal, config, s.txpool, s.network, s.blockchain, s.executor, s.grpcServer, s.logger.Named("consensus"))
	if err != nil {
		return err
	}
	s.consensus = consensus

	return nil
}

type jsonRPCHub struct {
	state state.State

	*blockchain.Blockchain
	*txpool.TxPool
	*state.Executor
}

// HELPER + WRAPPER METHODS //

func (j *jsonRPCHub) getState(root types.Hash, slot []byte) ([]byte, error) {
	// the values in the trie are the hashed objects of the keys
	key := keccak.Keccak256(nil, slot)

	snap, err := j.state.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}
	result, ok := snap.Get(key)
	if !ok {
		return nil, jsonrpc.ErrStateNotFound
	}
	return result, nil
}

func (j *jsonRPCHub) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	obj, err := j.getState(root, addr.Bytes())
	if err != nil {
		return nil, err
	}
	var account state.Account
	if err := account.UnmarshalRlp(obj); err != nil {
		return nil, err
	}
	return &account, nil
}

// GetForksInTime returns the active forks at the given block height
func (j *jsonRPCHub) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return j.Executor.GetForksInTime(blockNumber)
}

func (j *jsonRPCHub) GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	account, err := j.GetAccount(root, addr)

	if err != nil {
		return nil, err
	}

	obj, err := j.getState(account.Root, slot.Bytes())

	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (j *jsonRPCHub) GetCode(hash types.Hash) ([]byte, error) {
	res, ok := j.state.GetCode(hash)

	if !ok {
		return nil, fmt.Errorf("unable to fetch code")
	}

	return res, nil
}

func (j *jsonRPCHub) ApplyTxn(header *types.Header, txn *types.Transaction) (result *runtime.ExecutionResult, err error) {
	blockCreator, err := j.GetConsensus().GetBlockCreator(header)
	if err != nil {
		return nil, err
	}

	transition, err := j.BeginTxn(header.StateRoot, header, blockCreator)

	if err != nil {
		return
	}

	result, err = transition.Apply(txn)

	return
}

// SETUP //

// setupJSONRCP sets up the JSONRPC server, using the set configuration
func (s *Server) setupJSONRPC() error {
	hub := &jsonRPCHub{
		state:      s.state,
		Blockchain: s.blockchain,
		TxPool:     s.txpool,
		Executor:   s.executor,
	}

	conf := &jsonrpc.Config{
		Store:   hub,
		Addr:    s.config.JSONRPCAddr,
		ChainID: uint64(s.config.Chain.Params.ChainID),
	}

	srv, err := jsonrpc.NewJSONRPC(s.logger, conf)
	if err != nil {
		return err
	}
	s.jsonrpcServer = srv

	return nil
}

// setupGRPC sets up the grpc server and listens on tcp
func (s *Server) setupGRPC() error {
	proto.RegisterSystemServer(s.grpcServer, &systemService{s: s})

	lis, err := net.Listen("tcp", s.config.GRPCAddr.String())
	if err != nil {
		return err
	}

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			s.logger.Error(err.Error())
		}
	}()

	s.logger.Info("GRPC server running", "addr", s.config.GRPCAddr.String())

	return nil
}

// Chain returns the chain object of the client
func (s *Server) Chain() *chain.Chain {
	return s.chain
}

func (s *Server) Join(addr0 string, dur time.Duration) error {
	return s.network.JoinAddr(addr0, dur)
}

// Close closes the Minimal server (blockchain, networking, consensus)
func (s *Server) Close() {
	// Close the blockchain layer
	if err := s.blockchain.Close(); err != nil {
		s.logger.Error("failed to close blockchain", "err", err.Error())
	}

	// Close the networking layer
	if err := s.network.Close(); err != nil {
		s.logger.Error("failed to close networking", "err", err.Error())
	}

	// Close the consensus layer
	if err := s.consensus.Close(); err != nil {
		s.logger.Error("failed to close consensus", "err", err.Error())
	}

	// Close the state storage
	if err := s.stateStorage.Close(); err != nil {
		s.logger.Error("failed to close storage for trie", "err", err.Error())
	}
}

// Entry is a backend configuration entry
type Entry struct {
	Enabled bool
	Config  map[string]interface{}
}

// SetupDataDir sets up the polygon-sdk data directory and sub-folders
func SetupDataDir(dataDir string, paths []string) error {
	if err := createDir(dataDir); err != nil {
		return fmt.Errorf("Failed to create data dir: (%s): %v", dataDir, err)
	}

	for _, path := range paths {
		path := filepath.Join(dataDir, path)
		if err := createDir(path); err != nil {
			return fmt.Errorf("Failed to create path: (%s): %v", path, err)
		}
	}

	return nil
}

// createDir creates a file system directory if it doesn't exist
func createDir(path string) error {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

/// it is here temproarily to avoid cycle

func readValidatorsByRegexp(prefix string) ([]types.Address, error) {
	validators := []types.Address{}

	files, err := ioutil.ReadDir(".")
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		path := file.Name()
		if !file.IsDir() {
			continue
		}
		if !strings.HasPrefix(path, prefix) {
			continue
		}

		// try to read key from the filepath/consensus/<key> path
		possibleConsensusPath := filepath.Join(path, "consensus", ibft.IbftKeyName)

		// check if path exists
		if _, err := os.Stat(possibleConsensusPath); os.IsNotExist(err) {
			continue
		}

		priv, err := crypto.GenerateOrReadPrivateKey(possibleConsensusPath)
		if err != nil {
			return nil, err
		}
		validators = append(validators, crypto.PubKeyToAddress(&priv.PublicKey))
	}

	return validators, nil
}
