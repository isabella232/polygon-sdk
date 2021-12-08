package polybft

import (
	goproto "google.golang.org/protobuf/proto"
)

// Transport is a raw transport protocol to gossip messages
type Transport interface {
	NewTopic(name string, i goproto.Message) (Topic, error)
}

// Topic is a topic in the gossip network
type Topic interface {
	Publish(obj goproto.Message) error
	Subscribe(handler func(interface{})) error
}

// TODO: Add a transport wrapper that signs and validate the data
