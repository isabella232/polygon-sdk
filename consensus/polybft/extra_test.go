package polybft

import (
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-sdk/types"
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
