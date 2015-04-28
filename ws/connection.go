package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	_ "log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type HandlerFunc func(*Connection)

type HandshakeFunc func(*HttpRequest, *HttpResponse) HandlerFunc

type Connection struct {
	server     *Server
	r          *bufio.Reader
	w          *bufio.Writer
	conn       net.Conn
	Extensions []string
	MaxMsgLen  int
	inFrame    *Frame // current incoming frame
	outOpcode  uint8  // current opcode for Write call
	rcvdClose  uint16
}

const (
	OPCODE_CONTINUATION = 0
	OPCODE_TEXT         = 1
	OPCODE_BINARY       = 2
	OPCODE_CLOSE        = 8
	OPCODE_PING         = 9
	OPCODE_PONG         = 10
	_OPCODE_FAKE        = 0xFF
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
	KeyMagic         = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	MaxMsgLen        = 1024 * 1024 // TODO: move to server
	ReadBufferSize   = 1 * 1024    // TODO: increase, move to server
	WriteBufferSize  = 1 * 1024    // TODO: increase, move to server
	CloseWaitTimeout = 5
)

var (
	ErrMsgTooLong = errors.New("incoming frame is too long")
	EndOfFrame    = errors.New("end of frame")
	EndOfMessage  = errors.New("end of message")
)

func acceptKey(key string) string {
	buf := make([]byte, len(key)+len(KeyMagic))
	copy(buf, key)
	copy(buf[len(key):], KeyMagic)
	buf2 := sha1.Sum(buf)
	return base64.StdEncoding.EncodeToString(buf2[:])
}

func newConnection(server *Server, conn net.Conn) *Connection {
	var wsc Connection
	wsc.server = server
	wsc.conn = conn
	wsc.r = bufio.NewReaderSize(conn, ReadBufferSize)
	wsc.w = bufio.NewWriterSize(conn, WriteBufferSize)
	wsc.MaxMsgLen = MaxMsgLen
	wsc.outOpcode = _OPCODE_FAKE
	return &wsc
}

func (wsc *Connection) serve() {
	req := newHttpRequest()
	rsp := newHttpResponse()
	err := req.ReadFrom(wsc.r)
	if err != nil {
		rsp.Status = http.StatusBadRequest
		rsp.Headers["Content-Type"] = "text/plain"
		rsp.Headers["Connection"] = "close"
		rsp.WriteTo(wsc.w)
		wsc.conn.Close()
		return
	}
	handler := wsc.httpHandshake(req, rsp)
	if handler == nil {
		rsp.Headers["Content-Type"] = "text/plain"
		rsp.Headers["Connection"] = "close"
		rsp.WriteTo(wsc.w)
		wsc.conn.Close()
		return
	} else {
		rsp.WriteTo(wsc.w)
	}
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
	if req.Headers["Sec-Websocket-Key"] == "" {
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
	handler := wsc.server.Handshake(req, rsp)
	if handler != nil {
		rsp.Status = http.StatusSwitchingProtocols
		rsp.Headers["Upgrade"] = "websocket"
		rsp.Headers["Connection"] = "Upgrade"
		rsp.Headers["Sec-WebSocket-Accept"] = acceptKey(req.Headers["Sec-Websocket-Key"])
	}
	return handler
}

//////////////// Read - Write interface ////////////////////

func (wsc *Connection) Read(b []byte) (int, error) {
	var err error
	if wsc.inFrame == nil {
		wsc.inFrame = newFrame(wsc)
		if err = wsc.inFrame.readHeader(); err != nil {
			wsc.inFrame = nil
			return 0, err
		}
	}
	n, err := wsc.inFrame.read(b)
	if err == EndOfFrame {
		if wsc.inFrame.Fin {
			err = EndOfMessage
		} else {
			err = nil
		}
	}
	return n, err
}

func (wsc *Connection) StartWrite(text bool) {
	if wsc.outOpcode != _OPCODE_FAKE {
		panic("StartWrtite called without closing previous write sequence")
	}
	if text {
		wsc.outOpcode = OPCODE_TEXT
	} else {
		wsc.outOpcode = OPCODE_BINARY
	}
}

func (wsc *Connection) Write(b []byte) (int, error) {
	if wsc.outOpcode != OPCODE_TEXT && wsc.outOpcode != OPCODE_BINARY && wsc.outOpcode != OPCODE_CONTINUATION {
		panic("Write called with incorrect opcode. Make sure StartWirte called before")
	}
	f := newFrame(wsc)
	f.Len = len(b)
	f.Opcode = wsc.outOpcode
	if wsc.outOpcode == OPCODE_TEXT || wsc.outOpcode == OPCODE_BINARY {
		wsc.outOpcode = OPCODE_CONTINUATION
	}
	err := f.writeHeader()
	if err != nil {
		return 0, err
	}
	return f.write(b)
}

func (wsc *Connection) StopWrite() error {
	f := newFrame(wsc)
	f.Len = 0
	f.Fin = true
	f.Opcode = OPCODE_CONTINUATION
	err := f.writeHeader()
	if err != nil {
		return err
	}
	wsc.w.Flush()
	wsc.outOpcode = _OPCODE_FAKE
	return nil
}

//////////////// Recv - Send interface ////////////////////

func (wsc *Connection) Recv() ([]byte, error) {
	bbuf := make([][]byte, 0, 10)
	total := 0
	for {
		f := newFrame(wsc)
		err := f.readHeader()
		if err != nil {
			return nil, err
		}
		//log.Printf("recv frame %s %s", f, err)
		if f.Opcode == OPCODE_PING {
			_, err := f.recv()
			if err != nil {
				return nil, err
			}
			wsc.sendPongFrame()
			continue
		} else if f.Opcode == OPCODE_CLOSE {
			b, err := f.recv()
			if err == nil && len(b) >= 2 {
				wsc.rcvdClose = uint16(b[0])<<8 + uint16(b[1])
				wsc.CloseWithCode(wsc.rcvdClose, "")
			} else {
				wsc.CloseWithCode(STATUS_PROTOCOL_ERROR, "")
			}
			return nil, io.EOF
		}

		if f.Len+total > wsc.MaxMsgLen {
			return nil, ErrMsgTooLong
		}
		b, err := f.recv()
		//log.Printf("recv frame body %d %s", len(b), err)
		if err != nil {
			return nil, err
		}
		bbuf = append(bbuf, b)
		total += len(b)
		if f.Fin {
			break
		}
	}
	res := make([]byte, total)
	total = 0
	for _, b := range bbuf {
		copy(res[total:], b)
		total += len(b)
	}
	return res, nil
}

func (wsc *Connection) send(b []byte, text bool) error {
	f := newFrame(wsc)
	f.Len = len(b)
	if text {
		f.Opcode = OPCODE_TEXT
	} else {
		f.Opcode = OPCODE_BINARY
	}
	f.Fin = true
	err := f.writeHeader()
	if err != nil {
		return err
	}
	_, err = f.write(b)
	if err != nil {
		return err
	}
	return wsc.w.Flush()
}

func (wsc *Connection) Send(b []byte) error {
	return wsc.send(b, false)
}

func (wsc *Connection) SendText(b []byte) error {
	return wsc.send(b, true)
}

func (wsc *Connection) sendPongFrame() error {
	f := newFrame(wsc)
	f.Fin = true
	f.Opcode = OPCODE_PONG
	err := f.writeHeader()
	if err != nil {
		return err
	}
	return wsc.w.Flush()
}

func (wsc *Connection) sendCloseFrame(code uint16, reason string) error {
	f := newFrame(wsc)
	f.Fin = true
	f.Opcode = OPCODE_CLOSE
	f.Len = 2 + len(reason)
	err := f.writeHeader()
	if err != nil {
		return err
	}
	b := make([]byte, f.Len)
	b[0] = byte((code >> 8) & 0xFF)
	b[1] = byte(code & 0xFF)
	copy(b[2:], reason)
	err = f.send(b)
	if err != nil {
		return err
	}
	return wsc.w.Flush()
}

func (wsc *Connection) Close() error {
	var st uint16 = STATUS_OK
	if wsc.rcvdClose > 0 {
		st = wsc.rcvdClose
	}
	return wsc.CloseWithCode(st, "")
}

func (wsc *Connection) CloseWithCode(code uint16, reason string) error {
	err := wsc.sendCloseFrame(code, reason)
	if err != nil {
		// TODO: log error
		wsc.conn.Close()
		return err
	}
	if wsc.rcvdClose > 0 {
		wsc.conn.Close()
		return nil
	}
	// await for close from client
	wsc.conn.SetReadDeadline(time.Now().Add(CloseWaitTimeout * time.Second))
	for {
		f := newFrame(wsc)
		err := f.readHeader()
		if err != nil {
			wsc.conn.Close()
			if err == io.EOF {
				return nil
			} else {
				return err
			}
		}
		if f.Opcode == OPCODE_CLOSE {
			wsc.conn.Close()
			return nil
		}
	}
	return nil
}
