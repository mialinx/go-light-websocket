package websocket

// simple message

type Message struct {
	Opcode uint8
	Body   []byte
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

func BuildCloseBody(code uint16, reason string) []byte {
	b := make([]byte, 2+len(reason))
	b[0] = byte((code >> 8) & 0xFF)
	b[1] = byte(code & 0xFF)
	copy(b[2:], reason)
	return b
}

func Err2Close(err error) []byte {
	if err == nil {
		return BuildCloseBody(STATUS_OK, "")
	}
	switch err {
	case ErrBadFrame, ErrUnmaskedFrame, ErrUnexpectedFrame, ErrUnexpectedContinuation:
		return BuildCloseBody(STATUS_PROTOCOL_ERROR, err.Error())
	case ErrUnknownOpcode:
		return BuildCloseBody(STATUS_UNACCEPTABLE_DATA, err.Error())
	case ErrMessageTooLarge:
		return BuildCloseBody(STATUS_TOO_LARGE, err.Error())
	default:
		return BuildCloseBody(STATUS_INTERNAL, "internal")
	}
	panic("freak out")
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
