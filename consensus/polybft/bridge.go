package polybft

import (
	"encoding/json"

	"github.com/0xPolygon/polygon-sdk/consensus/polybft/proto"
	"github.com/golang/protobuf/ptypes/any"
)

const messagePoolProto = "/pool/0.1"

type messagePoolTransport struct {
	i *PolyBFT

	topic Topic
}

func (m *messagePoolTransport) init() error {
	// register the gossip protocol
	// V3NOTE: We can piggyback the proto message for ibft though later one we can change it
	topic, err := m.i.network.NewTopic(messagePoolProto, &proto.MessageReq{})
	if err != nil {
		return err
	}
	m.topic = topic

	topic.Subscribe(func(obj interface{}) {
		val := obj.(*proto.MessageReq)

		var msg *Message
		if err := json.Unmarshal(val.Proposal.Value, &msg); err != nil {
			panic(err)
		}
		m.i.pool.Add(msg, false)
	})
	return nil
}

func (m *messagePoolTransport) Gossip(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	req := &proto.MessageReq{
		Proposal: &any.Any{
			Value: data,
		},
	}
	m.topic.Publish(req)
}
