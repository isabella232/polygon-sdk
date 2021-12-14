package polybft

import (
	"fmt"

	"github.com/0xPolygon/polygon-sdk/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/wallet"
)

func commitMsg(b []byte) []byte {
	// message that the nodes need to sign to commit to a block
	// hash with COMMIT_MSG_CODE which is the same value used in quorum
	return web3.Keccak256(b, []byte{byte(proto.MessageReq_Commit)})
}

func ecrecoverImpl(sig, msg []byte) (types.Address, error) {
	pub, err := wallet.RecoverPubkey(sig, web3.Keccak256(msg))
	if err != nil {
		return types.Address{}, err
	}
	return crypto.PubKeyToAddress(pub), nil
}

/*
func ecrecoverFromHeader(h *types.Header) (types.Address, error) {
	// get the extra part that contains the seal
	extra, err := GetIbftExtra(h)
	if err != nil {
		return types.Address{}, err
	}
	// get the sig
	msg, err := calculateHeaderHash(h)
	if err != nil {
		return types.Address{}, err
	}

	return ecrecoverImpl(extra.Seal, msg)
}
*/

/*
func signSealImpl(prv web3.Key, h *types.Header, committed bool) ([]byte, error) {
	hash, err := calculateHeaderHash(h)
	if err != nil {
		return nil, err
	}

	// if we are singing the commited seals we need to do something more
	msg := hash
	if committed {
		msg = commitMsg(hash)
	}
	seal, err := prv.Sign(web3.Keccak256(msg))
	if err != nil {
		return nil, err
	}

	return seal, nil
}
*/

func writeSeal(prv web3.Key, hash []byte, committed bool) ([]byte, error) {
	msg := hash
	if committed {
		msg = commitMsg(hash)
	}
	seal, err := prv.Sign(web3.Keccak256(msg))
	if err != nil {
		return nil, err
	}
	return seal, nil
}

/*
func writeSeal(prv web3.Key, h *types.Header) (*types.Header, error) {
	h = h.Copy()
	seal, err := signSealImpl(prv, h, false)
	if err != nil {
		return nil, err
	}

	extra, err := GetIbftExtra(h)
	if err != nil {
		return nil, err
	}

	extra.Seal = seal
	if err := PutIbftExtra(h, extra); err != nil {
		return nil, err
	}

	return h, nil
}
*/

/*
func writeCommittedSeal2(prv web3.Key, h *types.Header) ([]byte, error) {
	return signSealImpl(prv, h, true)
}

func writeCommittedSeals2(h *types.Header, seals [][]byte) (*types.Header, error) {
	h = h.Copy()

	if len(seals) == 0 {
		return nil, fmt.Errorf("empty committed seals")
	}

	for _, seal := range seals {
		if len(seal) != ExtraSeal {
			return nil, fmt.Errorf("invalid committed seal length")
		}
	}

	extra, err := GetIbftExtra(h)
	if err != nil {
		return nil, err
	}

	extra.CommittedSeal = seals
	if err := PutIbftExtra(h, extra); err != nil {
		return nil, err
	}

	return h, nil
}
*/

func verifySigner(snap ValidatorSet, header *Header) error {
	signer, err := header.Ecrecover()
	if err != nil {
		return err
	}

	if !snap.Includes(signer) {
		return fmt.Errorf("not found signer")
	}

	return nil
}

// verifyCommitedFields is checking for consensus proof in the header
func verifyCommitedFields(snap ValidatorSet, header *Header) error {
	// Committed seals shouldn't be empty
	if len(header.Extra.CommittedSeal) == 0 {
		return fmt.Errorf("empty committed seals")
	}

	//fmt.Println("-- hash --")
	//fmt.Println(header)
	//fmt.Println(hash)

	rawMsg := commitMsg(header.Hash[:])

	//fmt.Println("-- raw validate msg --")
	//fmt.Println(rawMsg)

	visited := map[types.Address]struct{}{}
	for _, seal := range header.Extra.CommittedSeal {
		addr, err := ecrecoverImpl(seal, rawMsg)
		if err != nil {
			return err
		}

		if _, ok := visited[addr]; ok {
			return fmt.Errorf("repeated seal")
		} else {
			if !snap.Includes(addr) {
				return fmt.Errorf("signed by non validator: %s", addr.String())
			}
			visited[addr] = struct{}{}
		}
	}

	// Valid commited seals must be at least 2F+1
	// 	2F 	is the required number of honest validators who provided the commited seals
	// 	+1	is the proposer
	validSeals := len(visited)
	if validSeals <= 2*snap.MaxFaultyNodes() {
		return fmt.Errorf("not enough seals to seal block")
	}

	return nil
}

func validateMsg(msg *proto.MessageReq) error {
	signMsg, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}

	buf, err := hex.DecodeHex(msg.Signature)
	if err != nil {
		return err
	}

	addr, err := ecrecoverImpl(buf, signMsg)
	if err != nil {
		return err
	}

	msg.From = addr.String()

	return nil
}

func signMsg(key web3.Key, msg *proto.MessageReq) error {
	signMsg, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}

	sig, err := key.Sign(web3.Keccak256(signMsg))
	if err != nil {
		return err
	}

	msg.Signature = hex.EncodeToHex(sig)

	return nil
}
