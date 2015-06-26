package websocket

import (
	"fmt"
)

// simple message

type Message struct {
	Opcode uint8
	Body   []byte
}

func (m Message) String() string {
	name := OpcodeNames[m.Opcode]
	if name == "" {
		name = fmt.Sprintf("opcode_%d", m.Opcode)
	}
	format := "%x"
	if m.Opcode == OPCODE_TEXT {
		format = "%s"
	}
	const maxlen = 30
	body := m.Body
	cont := ""
	if len(body) > maxlen {
		body = body[0:maxlen]
		cont = "..."
	}
	return fmt.Sprintf(name+":"+format+cont, body)
}

func (m Message) Error() string {
	if name, ok := OpcodeNames[m.Opcode]; ok {
		return "<" + name + "> message"
	} else {
		return "<unknown> message"
	}
}

func ParseCloseBody(b []byte) (code uint16, reason string) {
	if len(b) >= 2 {
		code = uint16(b[0])<<8 + uint16(b[1])
	} else {
		code = STATUS_NOSTATUS
	}
	if len(b) > 2 {
		reason = string(b[2:])
	}
	return
}

func Err2CodeReason(err error) (uint16, string) {
	if err == nil {
		return STATUS_OK, ""
	}
	switch err {
	case ErrBadFrame, ErrUnmaskedFrame, ErrUnexpectedFrame, ErrUnexpectedContinuation:
		return STATUS_PROTOCOL_ERROR, err.Error()
	case ErrUnknownOpcode:
		return STATUS_UNACCEPTABLE_DATA, err.Error()
	case ErrMessageTooLarge:
		return STATUS_TOO_LARGE, err.Error()
	default:
		return STATUS_INTERNAL, "internal"
	}
	panic("freak out")
}

func BuildCloseBody(code uint16, reason string) []byte {
	b := make([]byte, 2+len(reason))
	b[0] = byte((code >> 8) & 0xFF)
	b[1] = byte(code & 0xFF)
	copy(b[2:], reason)
	return b
}

func BuildCloseBodyError(err error) []byte {
	return BuildCloseBody(Err2CodeReason(err))
}

// multiframe message

type MultiframeMessage struct {
	Opcode uint8
	Bbuf   [][]byte
}

func (m MultiframeMessage) Len() (total int) {
	for _, b := range m.Bbuf {
		total += len(b)
	}
	return
}

func (m *MultiframeMessage) Append(b []byte) {
	m.Bbuf = append(m.Bbuf, b)
}

func (m *MultiframeMessage) AsMessage() *Message {
	res := make([]byte, m.Len())
	bp := 0
	for _, b := range m.Bbuf {
		copy(res[bp:], b)
		bp += len(b)
	}
	return &Message{m.Opcode, res}
}
