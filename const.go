package websocket

import (
	"time"
)

const (
	KeyMagic               = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	DefaultMaxMsgLen       = 1024 * 1024
	DefaultReadBufferSize  = 2 * 1024
	DefaultWriteBufferSize = 2 * 1024
	MaxControlFrameLength  = 125
	AcceptErrorTimeout     = time.Second
	CloseWaitTimeout       = 5 * time.Second
)

const (
	OPCODE_CONTINUATION = 0
	OPCODE_TEXT         = 1
	OPCODE_BINARY       = 2
	OPCODE_CLOSE        = 8
	OPCODE_PING         = 9
	OPCODE_PONG         = 10
)

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
	STATUS_TOO_BIG           = 1009
	STATUS_NEED_EXTENSION    = 1010
	STATUS_INTERNAL          = 1011
)
