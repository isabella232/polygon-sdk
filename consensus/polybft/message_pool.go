package polybft

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"log"

	"github.com/0xPolygon/pbft-consensus"
)

type Message struct {
	// Hash is the hash of the data
	Hash string

	// Arbitrary data to be pooled
	Data []byte

	// From is the sender of the message
	From pbft.NodeID
}

type MessagePool struct {
	logger       *log.Logger
	local        pbft.NodeID
	transport    poolTransport
	messages     map[string]*messageTally
	validatorSet pbft.ValidatorSet
}

func NewMessagePool(logger *log.Logger, local pbft.NodeID, transport poolTransport) *MessagePool {
	if logger == nil {
		logger = log.New(ioutil.Discard, "", 0)
	}
	pool := &MessagePool{
		logger:    logger,
		local:     local,
		transport: transport,
		messages:  make(map[string]*messageTally),
	}
	return pool
}

func (m *MessagePool) GetReady() []*Message {
	res := []*Message{}

	for _, msg := range m.messages {
		if msg.ready {
			msg := &Message{
				// note that initially this was a message to tally votes (since incldue the frrom adress)
				// we are just reusing it to send back the event
				Data: msg.proposal,
			}
			res = append(res, msg) // include int he message tally the raw proposal so that we have easy access
		}
	}

	// Do we have to remove the message from the pool now,
	// or we acknoledge later on that it has been included? This part does not sound deterministic, we should if possible at any time include everything
	return res
}

// NOTE: This is recursive (reset also uses Add) which makes it complex sometimes to figure out where to draw lines
// and see which parts does what.
func (m *MessagePool) Add(msg *Message, gossip bool) {
	// gossip
	// m.transport.Gossip(msg)

	if msg.From == "" {
		panic("NOT FOUND?")
	}

	if msg.Hash == "" {
		// just hash the data ourselves if nothing found
		h := sha1.New()
		h.Write(msg.Data)
		msg.Hash = hex.EncodeToString(h.Sum(nil))
	}

	// add to pool
	m.addImpl(msg)

	if gossip {
		m.transport.Gossip(msg)
	}
}

func (m *MessagePool) addImpl(msg *Message) {
	m.logger.Printf("[INFO] new message in the pool: from %s, hash %s", msg.From, msg.Hash)

	if !m.validatorSet.Includes(msg.From) {
		return
	}
	if msg.Hash == "" {
		// hash the message
		hashRaw := sha256.Sum256(msg.Data)
		msg.Hash = hex.EncodeToString(hashRaw[:])
	}
	tally, ok := m.messages[msg.Hash]
	if !ok {
		tally = newMessageTally(msg.Data)
		m.messages[msg.Hash] = tally
	}
	count := tally.addMsg(msg)
	if count > m.validatorSet.Len()/2 { // mock value (depending on validatorset)
		tally.ready = true
	}
}

func (m *MessagePool) Reset(validatorSet pbft.ValidatorSet) {
	m.validatorSet = validatorSet

	// remove the values from the pool but reschedule the ones we have seen
	reschedule := []*Message{}
	for id, tally := range m.messages {
		if msg, ok := tally.hasLocal(m.local); ok {
			reschedule = append(reschedule, msg)
		}
		delete(m.messages, id)
	}

	// Should we send this if we are not a validator anymore? TODO

	// send again the rescheduled messages
	for _, msg := range reschedule {
		m.addImpl(msg)
	}
}

type messageTally struct {
	// tally of seen messages
	tally map[pbft.NodeID]*Message

	// arbitrary bytes of the proposal
	proposal []byte

	// ready selects whether the message is valid
	ready bool
}

func newMessageTally(proposal []byte) *messageTally {
	return &messageTally{
		tally:    map[pbft.NodeID]*Message{},
		proposal: proposal,
	}
}

func (m *messageTally) addMsg(msg *Message) int {
	if _, ok := m.tally[msg.From]; !ok {
		m.tally[msg.From] = msg
	}
	return len(m.tally)
}

func (m *messageTally) hasLocal(local pbft.NodeID) (*Message, bool) {
	msg, ok := m.tally[local]
	return msg, ok
}

// 1. validate the message in the pool? i.e. it decodes to an Ethereum event.
// 2. message pool is the one connected with the Transport
// 3. signing is done already in the gossip protocol.
// 4. being able to reset the pool.
// 5. add storage to save the data while there are reorgs.
//		- Also useful to get stats of what is being send and when.
// 		- This should only be valid for local ones.
// 6. we have to store messages even if we do not have it yet.
//		- This should only be reset at each epoch.
// 7. Connected with a ValidatorSet to know if it is a valid message?
// 8. We have two types of messages, we need to abstract that (arbitrary bytes?)
// 9. If the validator set changes the threshold for message valid changes too.
// 10. Even when there are slashings do we have to reset the message tally?
// 		- that moment would be good to pass the validator set.

type poolTransport interface {
	Gossip(msg *Message)
}
