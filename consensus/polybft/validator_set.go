package polybft

import (
	"math"

	"github.com/0xPolygon/polygon-sdk/types"
)

type ValidatorSet []types.Address

// CalcProposer calculates the address of the next proposer, from the validator set
func (v *ValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
	seed := uint64(0)
	if lastProposer == types.ZeroAddress {
		seed = round
	} else {
		offset := 0
		if indx := v.Index(lastProposer); indx != -1 {
			offset = indx
		}

		seed = uint64(offset) + round + 1
	}

	pick := seed % uint64(v.Len())

	return (*v)[pick]
}

// Add adds a new address to the validator set
func (v *ValidatorSet) Add(addr types.Address) {
	*v = append(*v, addr)
}

// Del removes an address from the validator set
func (v *ValidatorSet) Del(addr types.Address) {
	for indx, i := range *v {
		if i == addr {
			*v = append((*v)[:indx], (*v)[indx+1:]...)
		}
	}
}

// Len returns the size of the validator set
func (v *ValidatorSet) Len() int {
	return len(*v)
}

// Equal checks if 2 validator sets are equal
func (v *ValidatorSet) Equal(vv *ValidatorSet) bool {
	if len(*v) != len(*vv) {
		return false
	}
	for indx := range *v {
		if (*v)[indx] != (*vv)[indx] {
			return false
		}
	}

	return true
}

// Index returns the index of the passed in address in the validator set.
// Returns -1 if not found
func (v *ValidatorSet) Index(addr types.Address) int {
	for indx, i := range *v {
		if i == addr {
			return indx
		}
	}

	return -1
}

// Includes checks if the address is in the validator set
func (v *ValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

// MaxFaultyNodes returns the maximum number of allowed faulty nodes, based on the current validator set
func (v *ValidatorSet) MaxFaultyNodes() int {
	// numberOfValidators / 3
	return int(math.Ceil(float64(len(*v))/3)) - 1
}
