package websocket

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
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
	LogLevel   uint8
	rcvdClose  uint16
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
	KeyMagic               = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	MaxControlFrameLength  = 125
	CloseWaitTimeout       = 5
	DefaultMaxMsgLen       = 1024 * 1024
	DefaultReadBufferSize  = 2 * 1024
	DefaultWriteBufferSize = 2 * 1024
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

	rbs := server.ReadBufferSize
	if rbs == 0 {
		rbs = DefaultReadBufferSize
	}
	wsc.r = bufio.NewReaderSize(conn, rbs)
	//wsc.conn.SetReadBuffer(rbs)

	wbs := server.WriteBufferSize
	if wbs == 0 {
		wbs = DefaultWriteBufferSize
	}
	wsc.w = bufio.NewWriterSize(conn, wbs)
	//wsc.conn.SetWriteBuffer(wbs)

	mml := server.MaxMsgLen
	if mml == 0 {
		mml = DefaultMaxMsgLen
	}
	wsc.MaxMsgLen = mml
	return &wsc
}

func (wsc *Connection) serve() {
	req := newHttpRequest()
	rsp := newHttpResponse()
	err := req.ReadFrom(wsc.r)
	if err != nil {
		wsc.LogError("http parse %s", err)
		rsp.Status = http.StatusBadRequest
		rsp.Headers["Content-Type"] = "text/plain"
		rsp.Headers["Connection"] = "close"
		rsp.WriteTo(wsc.w)
		wsc.conn.Close()
		return
	}
	handler := wsc.httpHandshake(req, rsp)
	if handler == nil {
		wsc.LogError("handshake failed %d: %s", rsp.Status, rsp.Body)
		rsp.Headers["Content-Type"] = "text/plain"
		rsp.Headers["Connection"] = "close"
		rsp.WriteTo(wsc.w)
		wsc.conn.Close()
		return
	} else {
		rsp.WriteTo(wsc.w)
	}
	wsc.LogInfo("connection established")
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

type MessageReader struct {
	wsc    *Connection
	frame  *Frame
	useEom bool
	closed bool
}

func (wsc *Connection) NewReader(useEom bool) *MessageReader {
	return &MessageReader{wsc: wsc, useEom: useEom}
}

func (mr *MessageReader) Read(b []byte) (int, error) {
	if mr.closed {
		return 0, io.EOF
	}
	if mr.frame == nil {
		mr.frame = newFrame(mr.wsc)
		// TODO: handle control frames
		if err := mr.frame.readHeader(); err != nil {
			mr.closed = true
			return 0, err
		}
		mr.wsc.LogDebug("frame header received: %s", mr.frame)
	}
	n, err := mr.frame.read(b)
	mr.wsc.LogDebug("frame body read: %d", n)
	if err == EndOfFrame {
		if mr.frame.Fin {
			if mr.useEom {
				err = EndOfMessage
			} else {
				err = io.EOF
			}
			mr.closed = true
		} else {
			err = nil
		}
	}
	return n, err
}

type MessageWriter struct {
	wsc    *Connection
	opcode uint8
	closed bool
}

func (wsc *Connection) NewWriter(binary bool) *MessageWriter {
	mw := &MessageWriter{wsc: wsc}
	if binary {
		mw.opcode = OPCODE_BINARY
	} else {
		mw.opcode = OPCODE_TEXT
	}
	return mw
}

func (mw *MessageWriter) Write(b []byte) (int, error) {
	if mw.closed {
		return 0, io.EOF
	}
	f := newFrame(mw.wsc)
	f.Len = len(b)
	f.Opcode = mw.opcode
	if mw.opcode == OPCODE_TEXT || mw.opcode == OPCODE_BINARY {
		mw.opcode = OPCODE_CONTINUATION
	}
	err := f.writeHeader()
	if err != nil {
		return 0, err
	}
	mw.wsc.LogDebug("frame header sent: %s", f)
	n, err := f.write(b)
	mw.wsc.LogDebug("frame body write: %d", n)
	return n, err
}

func (mw *MessageWriter) Close() error {
	mw.closed = true
	f := newFrame(mw.wsc)
	f.Len = 0
	f.Fin = true
	f.Opcode = OPCODE_CONTINUATION
	err := f.writeHeader()
	if err != nil {
		return err
	}
	mw.wsc.LogDebug("frame header sent: %s", f)
	mw.wsc.w.Flush()
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
		wsc.LogDebug("frame header received: %s", f)
		if f.Opcode == OPCODE_PING {
			b, err := f.recv()
			if err != nil {
				return nil, err
			}
			wsc.SendPong(b)
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
		if err != nil {
			return nil, err
		}
		if f.Len+total > wsc.MaxMsgLen {
			return nil, ErrMsgTooLong
		}
		b, err := f.recv()
		if err != nil {
			return nil, err
		}
		wsc.LogDebug("frame body received: %d", len(b))
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

func (wsc *Connection) send(opcode uint8, b []byte) error {
	f := newFrame(wsc)
	f.Len = len(b)
	f.Opcode = opcode
	if opcode == OPCODE_PING || opcode == OPCODE_PONG || opcode == OPCODE_CLOSE {
		if f.Len > MaxControlFrameLength {
			panic(fmt.Sprintf("control frame %d exceeds data max data length %d", f.Opcode, f.Len))
		}
	}
	f.Fin = true
	err := f.writeHeader()
	if err != nil {
		return err
	}
	wsc.LogDebug("frame header sent: %s", f)
	_, err = f.write(b)
	if err != nil {
		return err
	}
	wsc.LogDebug("frame body sent: %d", len(b))
	return wsc.w.Flush()
}

func (wsc *Connection) Send(b []byte) error {
	return wsc.send(OPCODE_BINARY, b)
}

func (wsc *Connection) SendText(b []byte) error {
	return wsc.send(OPCODE_TEXT, b)
}

func (wsc *Connection) SendPong(b []byte) error {
	return wsc.send(OPCODE_PONG, b)
}

func (wsc *Connection) SendPing(b []byte) error {
	return wsc.send(OPCODE_PING, b)
}

func (wsc *Connection) sendClose(code uint16, reason string) error {
	b := make([]byte, 2+len(reason))
	b[0] = byte((code >> 8) & 0xFF)
	b[1] = byte(code & 0xFF)
	copy(b[2:], reason)
	return wsc.send(OPCODE_CLOSE, b)
}

func (wsc *Connection) Close() error {
	var st uint16 = STATUS_OK
	if wsc.rcvdClose > 0 {
		st = wsc.rcvdClose
	}
	return wsc.CloseWithCode(st, "")
}

func (wsc *Connection) CloseWithCode(code uint16, reason string) error {
	err := wsc.sendClose(code, reason)
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
	wsc.SetReadDeadlineD(CloseWaitTimeout * time.Second)
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

//////////////// Options ////////////////////

func (wsc *Connection) SetReadDeadline(t time.Time) error {
	return wsc.conn.SetReadDeadline(t)
}

func (wsc *Connection) SetReadDeadlineD(d time.Duration) error {
	return wsc.conn.SetReadDeadline(time.Now().Add(d))
}

func (wsc *Connection) SetWriteDeadline(t time.Time) error {
	return wsc.conn.SetWriteDeadline(t)
}

func (wsc *Connection) SetWriteDeadlineD(d time.Duration) error {
	return wsc.conn.SetWriteDeadline(time.Now().Add(d))
}

//////////////// Logging ////////////////////

const (
	LOG_ERROR = 0
	LOG_WARN  = 1
	LOG_INFO  = 2
	LOG_DEBUG = 3
)

var logNames map[int]string = map[int]string{
	LOG_ERROR: "ERROR",
	LOG_WARN:  "WARN",
	LOG_INFO:  "INFO",
	LOG_DEBUG: "DEBUG",
}

func (wsc *Connection) Log(level uint8, fmt string, args ...interface{}) {
	if level > wsc.LogLevel {
		return
	}
	addr := wsc.conn.RemoteAddr()
	// facepalm
	all := make([]interface{}, 2+len(args))
	all[0] = addr
	all[1] = level
	copy(all[2:], args)
	// facepalm is over
	log.Printf("%s %s "+fmt+"\n", all...)
}

func (wsc *Connection) LogError(fmt string, args ...interface{}) {
	wsc.Log(LOG_ERROR, fmt, args)
}

func (wsc *Connection) LogWarn(fmt string, args ...interface{}) {
	wsc.Log(LOG_WARN, fmt, args)
}

func (wsc *Connection) LogInfo(fmt string, args ...interface{}) {
	wsc.Log(LOG_INFO, fmt, args)
}

func (wsc *Connection) LogDebug(fmt string, args ...interface{}) {
	wsc.Log(LOG_DEBUG, fmt, args)
}