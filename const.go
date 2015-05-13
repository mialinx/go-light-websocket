package websocket

import (
	"errors"
	"time"
)

const (
	KeyMagic                     = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	DefaultMaxMsgLen             = 1024 * 1024
	DefaultReadBufferSize        = 2 * 1024
	DefaultWriteBufferSize       = 2 * 1024
	MaxControlFrameLength        = 125
	AcceptErrorTimeout           = time.Second
	DefaultCloseTimeout          = 5 * time.Second
	DefaultHandshakeReadTimeout  = 3 * time.Second
	DefaultHandshakeWriteTimeout = 3 * time.Second
)

const (
	OPCODE_CONTINUATION = 0
	OPCODE_TEXT         = 1
	OPCODE_BINARY       = 2
	OPCODE_CLOSE        = 8
	OPCODE_PING         = 9
	OPCODE_PONG         = 10
)

var KnownOpcodes []uint8 = []uint8{
	OPCODE_CONTINUATION,
	OPCODE_TEXT,
	OPCODE_BINARY,
	OPCODE_CLOSE,
	OPCODE_PING,
	OPCODE_PONG,
}

var OpcodeNames map[uint8]string = map[uint8]string{
	OPCODE_CONTINUATION: "continuation",
	OPCODE_TEXT:         "text",
	OPCODE_BINARY:       "binary",
	OPCODE_CLOSE:        "close",
	OPCODE_PING:         "ping",
	OPCODE_PONG:         "pong",
}

const (
	STATUS_OK                = 1000
	STATUS_GOAWAY            = 1001
	STATUS_PROTOCOL_ERROR    = 1002
	STATUS_UNACCEPTABLE_DATA = 1003
	STATUS_RESERVED          = 1004
	STATUS_NOSTATUS          = 1005
	STATUS_BAD_CLOSED        = 1006
	STATUS_BAD_DATA          = 1007
	STATUS_POLICY            = 1008
	STATUS_TOO_LARGE         = 1009
	STATUS_NEED_EXTENSION    = 1010
	STATUS_INTERNAL          = 1011
)

var (
	EndOfFrame                = errors.New("end of frame")
	EndOfMessage              = errors.New("end of message")
	ErrBadFrame               = errors.New("bad frame")
	ErrUnmaskedFrame          = errors.New("unmasked frame")
	ErrUnexpectedFrame        = errors.New("unexpected text/binary frame in sequence")
	ErrUnexpectedContinuation = errors.New("unexpected continuation frame")
	ErrUnknownOpcode          = errors.New("frame with unknown opcode")
	ErrMessageTooLarge        = errors.New("message too large")
	ErrConnectionClosed       = errors.New("connection already closed")
	ErrMessageClosed          = errors.New("message already closed")
)
