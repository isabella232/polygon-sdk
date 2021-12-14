package polybft

import (
	"crypto/rand"
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestExtraEncoding(t *testing.T) {
	seal1 := types.StringToHash("1").Bytes()

	cases := []struct {
		extra []byte
		data  *Extra
	}{
		{
			data: &Extra{
				Validators: []types.Address{
					types.StringToAddress("1"),
				},
				Seal: seal1,
				CommittedSeal: [][]byte{
					seal1,
				},
			},
		},
	}

	for _, c := range cases {
		data := c.data.MarshalRLPTo(nil)

		ii := &Extra{}
		if err := ii.UnmarshalRLP(data); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(c.data, ii) {
			t.Fatal("bad")
		}
	}
}

func TestExtra_RlpFuzz(t *testing.T) {
	// TODO: Use fastrlp
}

func TestExtra_Ecrecover(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A")

	var hash web3.Hash
	rand.Read(hash[:])

	// sign the hash
	seal := pool.get("A").seal(hash[:])

	header := &Header{
		Hash: hash,
		Extra: &Extra{
			Seal: seal,
		},
	}
	addr, err := header.Ecrecover()
	assert.NoError(t, err)
	assert.Equal(t, pool.get("A").Address(), addr)
}
