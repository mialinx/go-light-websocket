package ws

import (
	"bufio"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type HandlerFunc func(*Connection)

type HandshakeFunc func(*HttpRequest, *HttpResponse) HandlerFunc

type Connection struct {
	server     *Server
	r          *bufio.Reader
	w          *bufio.Writer
	c          net.Conn
	Key        [16]byte
	Extensions []string
	MaxMsgLen  int
	inFrame    *Frame // current incoming frame
	outFrame   *Frame // first outgoing frame in message
	rcvdClose  uint16
}

type Frame struct {
	Fin    bool
	Opcode byte
	Mask   bool
	Key    [4]byte
	Len    int
	r      *bufio.Reader
	read   int
}

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

const (
	KEY_MAGIC          = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	MaxMsgLen          = 1024 * 1024 // TODO: move to server
	ReadBufferSize     = 4 * 1024    // TODO: increase, move to server
	CLOSE_WAIT_TIMEOUT = 5 * time.Second
)

var (
	ErrMsgTooLong = errors.New("incoming frame is too long")
	EndOfFrame    = errors.New("end of frame")
	EndOfMessage  = errors.New("end of message")
)

func acceptKey(key string) string {
	buf := make([]byte, len(key)+len(KEY_MAGIC))
	copy(buf, key)
	copy(buf[len(key):], KEY_MAGIC)
	return base64.StdEncoding.EncodeToString(buf)
}

func newConnection(server *Server, c net.Conn) *Connection {
	var wsc Connection
	wsc.server = server
	wsc.c = c
	wsc.r = bufio.NewReaderSize(c, ReadBufferSize)
	wsc.w = bufio.NewWriter(c)
	wsc.MaxMsgLen = MaxMsgLen
	return &wsc
}

func (wsc *Connection) serve() {
	log.Printf("serve started\n")
	req := newHttpRequest()
	rsp := newHttpResponse()
	err := req.ReadFrom(wsc.r)
	if err != nil {
		rsp.Status = http.StatusBadRequest
		rsp.Headers["Content-Type"] = "text/plain"
		rsp.Headers["Connection"] = "close"
		rsp.WriteTo(wsc.w)
		wsc.c.Close()
		return
	}
	handler := wsc.httpHandshake(req, rsp)
	if handler == nil {
		rsp.Headers["Content-Type"] = "text/plain"
		rsp.Headers["Connection"] = "close"
		rsp.WriteTo(wsc.w)
		wsc.c.Close()
		return
	} else {
		rsp.WriteTo(wsc.w)
	}
	log.Printf("handler is %v\n", handler)
	handler(wsc)
}

func (wsc *Connection) httpHandshake(req *HttpRequest, rsp *HttpResponse) HandlerFunc {
	if req.Method != "GET" {
		rsp.Status = http.StatusMethodNotAllowed
		return nil
	}
	upgrade := false
	for _, val := range strings.Split(req.Headers["Connection"], ",") {
		if strings.ToLower(strings.TrimSpace(val)) == "upgrade" {
			upgrade = true
		}
	}
	if !upgrade {
		rsp.Status = http.StatusBadRequest
		rsp.Body = "'Connection: Upgrade' header missed"
		return nil
	}
	if strings.ToLower(req.Headers["Upgrade"]) != "websocket" {
		rsp.Status = http.StatusBadRequest
		rsp.Body = "'Upgrade: websocket' header missed"
		return nil
	}
	if key := req.Headers["Sec-Websocket-Key"]; key != "" {
		key, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(key) != 16 {
			rsp.Status = http.StatusBadRequest
			rsp.Body = "Invalid 'Sec-WebSocket-Key' header value"
			return nil
		}
		copy(wsc.Key[:], key)
	} else {
		rsp.Status = http.StatusBadRequest
		rsp.Body = "'Sec-Websocket-Key' header missed"
		return nil
	}
	if val := req.Headers["Sec-Websocket-Version"]; val != "" {
		version, err := strconv.Atoi(val)
		if err != nil || version != 13 {
			rsp.Status = http.StatusBadRequest
			rsp.Body = "Invalid 'Sec-WebSocket-Version' header value (13 expected)"
			return nil
		}
	} else {
		rsp.Status = http.StatusBadRequest
		rsp.Body = "'Sec-WebSocket-Version' header missed"
		return nil
	}
	if val := req.Headers["Sec-Websocket-Extensions"]; val != "" {
		for _, val := range strings.Split(val, ",") {
			for _, ext := range strings.Split(val, ";") {
				if ext := strings.TrimSpace(ext); ext != "" {
					wsc.Extensions = append(wsc.Extensions, ext)
				}
			}
		}
		if len(wsc.Extensions) > 0 {
			// TODO: запилить экстеншенов что ли
		}
	}
	if origin := req.Headers["Origin"]; origin != "" && len(wsc.server.Origins) > 0 {
		// TODO: check CORS
	}
	handler := wsc.server.Handshake(req, rsp)
	if handler != nil {
		rsp.Status = http.StatusSwitchingProtocols
		rsp.Headers["Upgrade"] = "websocket"
		rsp.Headers["Connection"] = "Upgrade"
		rsp.Headers["Sec-WebSocket-Accept"] = acceptKey(req.Headers["Sec-Websocket-Key"])
	}
	return handler
}

func (wsc *Connection) readFrame() (*Frame, error) {
	var f Frame
	var b [8]byte
	if _, err := io.ReadFull(wsc.r, b[0:2]); err != nil {
		return nil, err
	}
	f.Fin = b[0]&0x01 > 0
	f.Opcode = b[0] >> 4
	f.Mask = b[1]&0x01 > 0
	f.Len = int(b[1] >> 1)
	if f.Len == 126 {
		if _, err := io.ReadFull(wsc.r, b[0:2]); err != nil {
			return nil, err
		}
		f.Len = 0
		f.Len += int(b[0] << 8)
		f.Len += int(b[1])
	} else if f.Len == 127 {
		if _, err := io.ReadFull(wsc.r, b[:]); err != nil {
			return nil, err
		}
		f.Len = 0
		f.Len += int(b[0] << 56)
		f.Len += int(b[1] << 48)
		f.Len += int(b[2] << 40)
		f.Len += int(b[3] << 32)
		f.Len += int(b[4] << 24)
		f.Len += int(b[5] << 16)
		f.Len += int(b[6] << 8)
		f.Len += int(b[7])
		if f.Len < 0 {
			return nil, ErrMsgTooLong
		}
	}
	if f.Mask {
		if _, err := io.ReadFull(wsc.r, f.Key[:]); err != nil {
			return nil, err
		}
	}
	f.r = wsc.r
	return &f, nil
}

func (wsc *Connection) writeFrame(f *Frame) error {
	b := make([]byte, 10)
	if f.Fin {
		b[0] |= 0x01
	}
	b[0] |= (f.Opcode << 4)
	if f.Len < 126 {
		b[1] = byte(f.Len << 1)
		b = b[0:2]
	} else if f.Len <= 0xFFFF {
		b[1] = 126
		b[2] = byte((f.Len >> 8) & 0xFF)
		b[3] = byte(f.Len & 0xFF)
		b = b[0:4]
	} else {
		b[1] = 127
		b[2] = byte((f.Len >> 56) & 0xFF)
		b[3] = byte((f.Len >> 48) & 0xFF)
		b[4] = byte((f.Len >> 40) & 0xFF)
		b[5] = byte((f.Len >> 32) & 0xFF)
		b[6] = byte((f.Len >> 24) & 0xFF)
		b[7] = byte((f.Len >> 16) & 0xFF)
		b[8] = byte((f.Len >> 8) & 0xFF)
		b[9] = byte(f.Len & 0xFF)
	}
	for len(b) > 0 {
		n, err := wsc.w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func (f *Frame) Read(b []byte) (int, error) {
	if f.read >= f.Len {
		return 0, EndOfFrame
	}
	if need := f.Len - f.read; len(b) > need {
		b = b[:need]
	}
	n, err := f.r.Read(b)
	for i := 0; i < n; i++ {
		b[i] ^= f.Key[(f.read+i)%4]
	}
	f.read += n
	if err != nil {
		return n, err
	}
	return n, nil
}

func (wsc *Connection) Read(b []byte) (int, error) {
	var err error
	if wsc.inFrame == nil {
		wsc.inFrame, err = wsc.readFrame()
		if err != nil {
			return 0, err
		}
	}
	n, err := wsc.inFrame.Read(b)
	if err == EndOfFrame {
		if wsc.inFrame.Fin {
			err = EndOfMessage
		} else {
			err = nil
		}
		return n, err
	}
	return n, err
}

func (wsc *Connection) Recv() ([]byte, error) {
	bbuf := make([][]byte, 0, 10)
	total := 0
	for {
		f, err := wsc.readFrame()
		log.Printf("frame %v err %v", f, err)
		if err != nil {
			return nil, err
		}
		if f.Len+total > wsc.MaxMsgLen {
			return nil, ErrMsgTooLong
		}
		buf := make([]byte, f.Len)
		read := 0
		for read < f.Len {
			n, err := f.Read(buf[read:])
			if err != nil {
				return nil, err
			}
			read += n
		}
		bbuf = append(bbuf, buf)
		total += len(buf)
		if f.Fin {
			break
		}
	}
	res := make([]byte, 0, total)
	total = 0
	for _, buf := range bbuf {
		copy(res[total:], buf)
		total += len(buf)
	}
	return res, nil
}

func (wsc *Connection) Write(b []byte, fin bool) error {
	var f Frame
	f.Len = len(b)
	f.Fin = fin
	if wsc.outFrame == nil {
		f.Opcode = OPCODE_BINARY // TODO: how to use ?
		wsc.outFrame = &f
	} else {
		f.Opcode = OPCODE_CONTINUATION
	}
	err := wsc.writeFrame(&f)
	if err != nil {
		return err
	}
	// TODO: writeAll
	for len(b) > 0 {
		n, err := wsc.w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	if fin {
		wsc.outFrame = nil
	}
	return nil
}

func (wsc *Connection) Send(msg []byte) error {
	return wsc.Write(msg, true)
}

func (wsc *Connection) Close() error {
	return wsc.CloseWithCode(STATUS_OK, "")
}

func (wsc *Connection) CloseWithCode(code uint16, reason string) error {
	err := wsc.writeCloseFrame(code, reason)
	if err != nil {
		// TODO: log error
		wsc.c.Close()
		return err
	}
	if wsc.rcvdClose > 0 {
		wsc.c.Close()
		return nil
	}
	// await for close from client
	wsc.c.SetReadDeadline(time.Now() + CLOSE_WAIT_TIMEOUT)
	for {
		f, err := wsc.readFrame()
		if err != nil {

		}
		if f != nil && f.Opcode == OPCODE_CLOSE {
			wsc.c.Close()
			return nil
		}
	}
	// TODO: from here
	if wsc.rcvdClose > 0 {
	}
}

func (wsc *Connection) writeCloseFrame(code uint16, reason string) error {
	var f Frame
	f.Fin = true
	f.Opcode = OPCODE_CLOSE
	f.Len = 2 + len(reason)
	err := wsc.writeFrame(&f)
	if err != nil {
		return err
	}
	b := make([]byte, f.Len)
	b[0] = byte((code >> 8) & 0xFF)
	b[1] = byte(code && 0xFF)
	copy(b[2:], reason)
	// TODO: writeAll
	for len(b) > 0 {
		n, err := wsc.w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}
