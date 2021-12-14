package polybft

import (
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/fastrlp"
	"github.com/umbracle/go-web3"
)

var (
	// PolyBFTDigest represents a hash of "PolyBFT" to identify whether the block is from PolyBFT consensus engine
	PolyBFTDigest = types.StringToHash("3ceb7974985f530ac20a5ddf2e9e9bd54d1674e810e06bbcca9015105aa6b6f9")

	// ExtraVanity represents a fixed number of extra-data bytes reserved for proposer vanity
	ExtraVanity = 32

	// ExtraSeal represents the fixed number of extra-data bytes reserved for proposer seal
	ExtraSeal = 65
)

// Extra defines the structure of the extra field for Istanbul
type Extra struct {
	Validators    []types.Address
	Seal          []byte
	CommittedSeal [][]byte
}

var zeroBytes = make([]byte, 32)

/*
// putIbftExtraValidators is a helper method that adds validators to the extra field in the header
func PutIbftExtraValidators(h *types.Header, validators []types.Address) {
	// Pad zeros to the right up to istanbul vanity
	extra := h.ExtraData
	if len(extra) < ExtraVanity {
		extra = append(extra, zeroBytes[:ExtraVanity-len(extra)]...)
	} else {
		extra = extra[:ExtraVanity]
	}

	ibftExtra := &Extra{
		Validators:    validators,
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
	}

	extra = ibftExtra.MarshalRLPTo(extra)
	h.ExtraData = extra
}
*/

func GetIbftExtraClean(extraB []byte) ([]byte, error) {
	extra, err := GetIbftExtra(extraB)
	if err != nil {
		return nil, err
	}

	ibftExtra := &Extra{
		Validators:    extra.Validators,
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
	}
	return ibftExtra.MarshalRLPTo(nil), nil
}

/*
// PutIbftExtra sets the extra data field in the header to the passed in istanbul extra data
func PutIbftExtra(h *types.Header, Extra *Extra) error {
	// Pad zeros to the right up to istanbul vanity
	extra := h.ExtraData
	if len(extra) < ExtraVanity {
		extra = append(extra, zeroBytes[:ExtraVanity-len(extra)]...)
	} else {
		extra = extra[:ExtraVanity]
	}

	data := Extra.MarshalRLPTo(nil)
	extra = append(extra, data...)
	h.ExtraData = extra

	return nil
}
*/

// getIbftExtra returns the istanbul extra data field from the passed in header
func GetIbftExtra(extraB []byte) (*Extra, error) {
	if len(extraB) < ExtraVanity {
		return nil, fmt.Errorf("wrong extra size: %d", len(extraB))
	}

	data := extraB[ExtraVanity:]
	extra := &Extra{}
	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

// MarshalRLPTo defines the marshal function wrapper for Extra
func (i *Extra) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(i.MarshalRLPWith, dst)
}

// MarshalRLPWith defines the marshal function implementation for Extra
func (i *Extra) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()

	// Validators
	vals := ar.NewArray()
	for _, a := range i.Validators {
		vals.Set(ar.NewBytes(a.Bytes()))
	}
	vv.Set(vals)

	// Seal
	if len(i.Seal) == 0 {
		vv.Set(ar.NewNull())
	} else {
		vv.Set(ar.NewBytes(i.Seal))
	}

	// CommittedSeal
	if len(i.CommittedSeal) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		committed := ar.NewArray()
		for _, a := range i.CommittedSeal {
			if len(a) == 0 {
				committed.Set(ar.NewNull())
			} else {
				committed.Set(ar.NewBytes(a))
			}
		}
		vv.Set(committed)
	}

	return vv
}

// UnmarshalRLP defines the unmarshal function wrapper for Extra
func (i *Extra) UnmarshalRLP(input []byte) error {
	return fastrlp.UnmarshalRLP(input, i)
}

// UnmarshalRLPFrom defines the unmarshal implementation for Extra
func (i *Extra) UnmarshalRLPWith(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if num := len(elems); num != 3 {
		return fmt.Errorf("not enough elements to decode extra, expected 3 but found %d", num)
	}

	// Validators
	{
		vals, err := elems[0].GetElems()
		if err != nil {
			return fmt.Errorf("list expected for validators")
		}
		i.Validators = make([]types.Address, len(vals))
		for indx, val := range vals {
			if err = val.GetAddr(i.Validators[indx][:]); err != nil {
				return err
			}
		}
	}

	// Seal
	{
		if i.Seal, err = elems[1].GetBytes(i.Seal); err != nil {
			return err
		}
	}

	// Committed
	{
		vals, err := elems[2].GetElems()
		if err != nil {
			return fmt.Errorf("list expected for committed")
		}
		i.CommittedSeal = make([][]byte, len(vals))
		for indx, val := range vals {
			if i.CommittedSeal[indx], err = val.GetBytes(i.CommittedSeal[indx]); err != nil {
				return err
			}
		}
	}
	return nil
}

// Header is a stub struct to represent a blockchain header in the Polybft protocol.
type Header struct {
	// Hash of the block.
	Hash web3.Hash

	// Hash of the parent block.
	ParentHash web3.Hash

	// Number of the block.
	Number uint64

	// Extra data unmarshalled. It does include fields not part of the
	// hash of the block.
	Extra *Extra

	// Timestamp when this block was sealed
	Timestamp time.Time
}

func (h *Header) Ecrecover() (types.Address, error) {
	return ecrecoverImpl(h.Extra.Seal, h.Hash[:])
}
