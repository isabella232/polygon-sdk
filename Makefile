
.PHONY: download-spec-tests
download-spec-tests:
	git submodule init
	git submodule update

.PHONY: bindata
bindata:
	go-bindata -pkg chain -o ./chain/chain_bindata.go ./chain/chains

.PHONY: protoc
protoc:
	protoc --go_out=. --go-grpc_out=. ./minimal/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./protocol/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./network/proto/test/*.proto
	protoc --go_out=. --go-grpc_out=. ./network/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./txpool/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./consensus/ibft/proto/*.proto

.PHONY: abigen
abigen:
	# You need to clone go-web3 and run "make abigen"
	# ../../umbracle/go-web3/abigen/bin/abigen --source ./contracts2/bridge.sol --output ./contracts2 --package contracts2
	../../umbracle/go-web3/abigen/bin/abigen --source ./smart-contract/artifacts/contracts/Bridge.sol/Bridge.json --output ./smart-contract/bindings --package bindings
