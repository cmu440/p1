// DO NOT MODIFY THIS FILE!

package lsp

import "fmt"

// MsgType is an integer code describing an LSP message type.
type MsgType int

const (
	MsgConnect MsgType = iota // Sent by clients to make a connection w/ the server.
	MsgData                   // Sent by clients/servers to send data.
	MsgAck                    // Sent by clients/servers to ack connect/data msgs.
)

// Message represents a message used by the LSP protocol.
type Message struct {
	Type    MsgType // One of the message types listed above.
	ConnID  int     // Unique client-server connection ID.
	SeqNum  int     // Message sequence number.
	Payload []byte  // Data message payload.
}

// NewConnect returns a new connect message.
func NewConnect() *Message {
	return &Message{Type: MsgConnect}
}

// NewData returns a new data message with the specified connection ID,
// sequence number, and payload.
func NewData(connID, seqNum int, payload []byte) *Message {
	return &Message{
		Type:    MsgData,
		ConnID:  connID,
		SeqNum:  seqNum,
		Payload: payload,
	}
}

// NewAck returns a new acknowledgement message with the specified
// connection ID and sequence number.
func NewAck(connID, seqNum int) *Message {
	return &Message{
		Type:   MsgAck,
		ConnID: connID,
		SeqNum: seqNum,
	}
}

// String returns a string representation of this message. To pretty-print a
// message, you can pass it to a format string like so:
//     msg := NewConnect()
//     fmt.Printf("Connect message: %s\n", msg)
func (m *Message) String() string {
	var name, payload string
	switch m.Type {
	case MsgConnect:
		name = "Connect"
	case MsgData:
		name = "Data"
		payload = " " + string(m.Payload)
	case MsgAck:
		name = "Ack"
	}
	return fmt.Sprintf("[%s %d %d%s]", name, m.ConnID, m.SeqNum, payload)
}
