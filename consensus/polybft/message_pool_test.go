package polybft

import (
	"testing"

	"github.com/0xPolygon/pbft-consensus"
)

func TestMessagePool_Insert(t *testing.T) {
	data1 := []byte{0x1}

	set := newMockValidatorSet([]string{"A", "B"})

	m := NewMessagePool(nil, pbft.NodeID("A"), nil)
	m.Reset(set)

	m.addImpl(&Message{
		Data: data1,
		From: pbft.NodeID("A"),
	})
}

type mockValidatorSet struct {
	ids []string
}

func newMockValidatorSet(ids []string) pbft.ValidatorSet {
	return &mockValidatorSet{ids: ids}
}

func (m *mockValidatorSet) CalcProposer(round uint64) pbft.NodeID {
	panic("not implemented")
}

func (m *mockValidatorSet) Includes(n pbft.NodeID) bool {
	for _, i := range m.ids {
		if i == string(n) {
			return true
		}
	}
	return false
}

func (m *mockValidatorSet) Len() int {
	return len(m.ids)
}
