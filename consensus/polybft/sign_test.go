package polybft

import (
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-sdk/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3/wallet"
)

type testerAccount struct {
	alias string
	priv  *wallet.Key
}

func (t *testerAccount) Address() types.Address {
	return types.Address(t.priv.Address())
}

func (t *testerAccount) sign(h *types.Header) *types.Header {
	h, _ = writeSeal(t.priv, h)
	return h
}

type testerAccountPool struct {
	accounts []*testerAccount
}

func newTesterAccountPool(num ...int) *testerAccountPool {
	t := &testerAccountPool{
		accounts: []*testerAccount{},
	}
	if len(num) == 1 {
		for i := 0; i < num[0]; i++ {
			key, _ := wallet.GenerateKey()
			t.accounts = append(t.accounts, &testerAccount{
				alias: strconv.Itoa(i),
				priv:  key,
			})
		}
	}
	return t
}

func (ap *testerAccountPool) add(accounts ...string) {
	for _, account := range accounts {
		if acct := ap.get(account); acct != nil {
			continue
		}
		priv, err := wallet.GenerateKey()
		if err != nil {
			panic("BUG: Failed to generate crypto key")
		}
		ap.accounts = append(ap.accounts, &testerAccount{
			alias: account,
			priv:  priv,
		})
	}
}

func (ap *testerAccountPool) get(name string) *testerAccount {
	for _, i := range ap.accounts {
		if i.alias == name {
			return i
		}
	}
	return nil
}

func (ap *testerAccountPool) ValidatorSet() ValidatorSet {
	v := ValidatorSet{}
	for _, i := range ap.accounts {
		v = append(v, i.Address())
	}
	return v
}

func TestSign_Sealer(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A")

	snap := pool.ValidatorSet()

	h := &types.Header{}
	putIbftExtraValidators(h, pool.ValidatorSet())

	// non-validator address
	pool.add("X")

	badSealedBlock, _ := writeSeal(pool.get("X").priv, h)
	assert.Error(t, verifySigner(snap, badSealedBlock))

	// seal the block with a validator
	goodSealedBlock, _ := writeSeal(pool.get("A").priv, h)
	assert.NoError(t, verifySigner(snap, goodSealedBlock))
}

func TestSign_CommittedSeals(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A", "B", "C", "D", "E")

	snap := pool.ValidatorSet()

	h := &types.Header{}
	putIbftExtraValidators(h, pool.ValidatorSet())

	// non-validator address
	pool.add("X")

	buildCommittedSeal := func(accnt []string) error {
		seals := [][]byte{}
		for _, accnt := range accnt {
			seal, err := writeCommittedSeal(pool.get(accnt).priv, h)
			assert.NoError(t, err)
			seals = append(seals, seal)
		}

		sealed, err := writeCommittedSeals(h, seals)
		assert.NoError(t, err)

		return verifyCommitedFields(snap, sealed)
	}

	// Correct
	assert.NoError(t, buildCommittedSeal([]string{"A", "B", "C"}))

	// Failed - Repeated signature
	assert.Error(t, buildCommittedSeal([]string{"A", "A"}))

	// Failed - Non validator signature
	assert.Error(t, buildCommittedSeal([]string{"A", "X"}))

	// Failed - Not enough signatures
	assert.Error(t, buildCommittedSeal([]string{"A"}))
}

func TestSign_Messages(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A")

	msg := &proto.MessageReq{}
	assert.NoError(t, signMsg(pool.get("A").priv, msg))
	assert.NoError(t, validateMsg(msg))

	assert.Equal(t, msg.From, pool.get("A").Address().String())
}
