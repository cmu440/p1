package bitcoin

import "fmt"

type MsgType int

const (
	Join MsgType = iota
	Request
	Result
)

// Message represents a message that can be sent between components in the bitcoin
// mining distributed system. Messages must be marshalled into a byte slice before being
// sent over the network.
type Message struct {
	Type         MsgType
	Data         string
	Lower, Upper uint64
	Hash, Nonce  uint64
}

// NewRequest creates a request message. Clients send request messages to the
// server and the server sends request messages to miners.
func NewRequest(data string, lower, upper uint64) *Message {
	return &Message{
		Type:  Request,
		Data:  data,
		Lower: lower,
		Upper: upper,
	}
}

// New result creates a result message. Miners send result messages to the server
// and the server sends result messages to clients.
func NewResult(hash, nonce uint64) *Message {
	return &Message{
		Type:  Result,
		Hash:  hash,
		Nonce: nonce,
	}
}

// NewJoin creates a join message. Miners send join messages to the server.
func NewJoin() *Message {
	return &Message{Type: Join}
}

func (m *Message) String() string {
	var result string
	switch m.Type {
	case Request:
		result = fmt.Sprintf("[%s %s %d %d]", "Request", m.Data, m.Lower, m.Upper)
	case Result:
		result = fmt.Sprintf("[%s %d %d]", "Result", m.Hash, m.Nonce)
	case Join:
		result = fmt.Sprintf("[%s]", "Join")
	}
	return result
}
