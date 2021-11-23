module github.com/0xPolygon/polygon-sdk

go 1.14

require (
	github.com/0xPolygon/pbft-consensus v0.0.0-20211117113442-6cb2c0968df0
	github.com/btcsuite/btcd v0.21.0-beta
	github.com/containerd/containerd v1.5.8 // indirect
	github.com/docker/docker v20.10.11+incompatible
	github.com/golang/protobuf v1.5.0
	github.com/google/gopacket v1.1.18 // indirect
	github.com/google/uuid v1.2.0
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-hclog v0.14.1
	github.com/hashicorp/go-immutable-radix v1.3.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/go-uuid v1.0.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/hashicorp/hcl v1.0.0
	github.com/imdario/mergo v0.3.12
	github.com/libp2p/go-libp2p v0.12.0
	github.com/libp2p/go-libp2p-core v0.7.0
	github.com/libp2p/go-libp2p-kbucket v0.4.7
	github.com/libp2p/go-libp2p-noise v0.1.1
	github.com/libp2p/go-libp2p-pubsub v0.4.1
	github.com/mitchellh/cli v1.0.0
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/ryanuber/columnize v2.1.2+incompatible
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
	github.com/umbracle/fastrlp v0.0.0-20210128110402-41364ca56ca8
	github.com/umbracle/go-eth-bn256 v0.0.0-20190607160430-b36caf4e0f6b
	github.com/umbracle/go-web3 v0.0.0-20211122090728-2060f20426ca
	go.uber.org/zap v1.16.0 // indirect
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/time v0.0.0-20211116232009-f0f3c7e86c11 // indirect
	google.golang.org/grpc v1.35.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
)

replace github.com/0xPolygon/pbft-consensus => ../../0xPolygon/pbft-consensus
