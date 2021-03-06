package websocket

import (
	"bufio"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

type HandlerFunc func(*Connection) error

type HandshakeFunc func(*Connection, *http.Request, http.ResponseWriter) HandlerFunc

type Connection struct {
	server     *Server
	conn       net.Conn
	r          *bufio.Reader
	w          *bufio.Writer
	Extensions []string
	LogLevel   uint8
	mm         *MultiframeMessage
	RcvdClose  *Message
	SentClose  *Message
	closed     bool
}

func acceptKey(key string) string {
	buf := make([]byte, len(key)+len(KeyMagic))
	copy(buf, key)
	copy(buf[len(key):], KeyMagic)
	buf2 := sha1.Sum(buf)
	return base64.StdEncoding.EncodeToString(buf2[:])
}

func newConnection(server *Server, conn net.Conn) *Connection {
	wsc := &Connection{
		server:   server,
		conn:     conn,
		LogLevel: server.Config.LogLevel,
	}
	var tconn *net.TCPConn
	switch conn := conn.(type) {
	case *net.TCPConn:
		tconn = conn
	case *tls.Conn:
		v := reflect.ValueOf(conn).Elem().FieldByName("conn")
		tconn = (*net.TCPConn)(unsafe.Pointer(v.Elem().Pointer()))
	default:
		panic("unexpected type of connection: neither TCP nor TLS")
	}
	tconn.SetReadBuffer(server.Config.SockReadBuffer)
	tconn.SetWriteBuffer(server.Config.SockWriteBuffer)
	if server.Config.TCPKeepAlive > 0 {
		tconn.SetKeepAlive(true)
		tconn.SetKeepAlivePeriod(server.Config.TCPKeepAlive)
	}
	wsc.setupBuffio(server.Config.HttpReadBuffer, server.Config.HttpWriteBuffer)
	return wsc
}

func (wsc *Connection) setupBuffio(rs, ws int) {
	var r io.Reader
	var w io.Writer
	if wsc.server.Config.IOStatistics {
		r = &ReaderWithStats{r: wsc.conn, stats: wsc.server.Stats}
		w = &WriterWithStats{w: wsc.conn, stats: wsc.server.Stats}
	} else {
		r = wsc.conn
		w = wsc.conn
	}
	wsc.r = bufio.NewReaderSize(r, rs)
	wsc.w = bufio.NewWriterSize(w, ws)
}

func (wsc *Connection) serve() {
	defer func() {
		if err := recover(); err != nil {
			wsc.LogError("panic: %s\n%s", err, debug.Stack())
		}
		if !wsc.closed {
			wsc.Close()
		}
		wsc.LogDebug("connection closed")
		wsc.server.Stats.add(eventClose{})
	}()
	wsc.LogDebug("connection established")
	wsc.server.Stats.add(eventConnect{})

	wsc.SetReadDeadlineDuration(wsc.server.Config.HandshakeReadTimeout)
	req, err := http.ReadRequest(wsc.r)
	wsc.SetReadDeadlineDuration(0)

	rspw := newHtttpResponseWriter()

	if err != nil {
		wsc.LogError("http parse %s", err)
		rspw.Header().Set("Content-Type", "text/plain")
		rspw.Header().Set("Connection", "close")
		rspw.WriteHeader(http.StatusBadRequest)
		wsc.SetWriteDeadlineDuration(wsc.server.Config.HandshakeWriteTimeout)
		rspw.WriteTo(wsc.w)
		wsc.w.Flush()
		wsc.SetWriteDeadlineDuration(0)
		wsc.Close()
		wsc.server.Stats.add(eventHandshakeFailed{})
		return
	}
	req.RemoteAddr = wsc.conn.RemoteAddr().String()
	handler := wsc.httpHandshake(req, rspw)
	if handler == nil {
		wsc.LogError("handshake failed %d: %s", rspw.rsp.StatusCode, rspw.body.String())
		rspw.Header().Set("Content-Type", "text/plain")
		rspw.Header().Set("Connection", "close")
		wsc.SetWriteDeadlineDuration(wsc.server.Config.HandshakeWriteTimeout)
		rspw.WriteTo(wsc.w)
		wsc.w.Flush()
		wsc.SetWriteDeadlineDuration(0)
		wsc.Close()
		wsc.server.Stats.add(eventHandshakeFailed{})
		return
	} else {
		wsc.SetWriteDeadlineDuration(wsc.server.Config.HandshakeWriteTimeout)
		rspw.WriteTo(wsc.w)
		wsc.w.Flush()
		wsc.SetWriteDeadlineDuration(0)
	}
	wsc.server.Stats.add(eventHandshake{})
	// let gc rip them
	req = nil
	rspw = nil

	// change bufferization
	if wsc.r.Buffered() > 0 {
		panic("unread data in buffer after http handshake")
	}
	wsc.w.Flush()
	wsc.setupBuffio(wsc.server.Config.WsReadBuffer, wsc.server.Config.WsWriteBuffer)

	// run ws
	err = handler(wsc)
	if err != nil && err != io.EOF {
		wsc.LogError("err: %T %s", err, err.Error())
	}
}

func (wsc *Connection) httpHandshake(req *http.Request, rspw http.ResponseWriter) HandlerFunc {
	if req.Method != "GET" {
		rspw.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	}
	upgrade := false
	for _, val := range strings.Split(req.Header.Get("Connection"), ", ") {
		if strings.ToLower(strings.TrimSpace(val)) == "upgrade" {
			upgrade = true
		}
	}
	if !upgrade {
		rspw.WriteHeader(http.StatusBadRequest)
		rspw.Write([]byte("'Connection: Upgrade' header missed"))
		return nil
	}
	if strings.ToLower(req.Header.Get("Upgrade")) != "websocket" {
		rspw.WriteHeader(http.StatusBadRequest)
		rspw.Write([]byte("'Upgrade: websocket' header missed"))
		return nil
	}
	if req.Header.Get("Sec-Websocket-Key") == "" {
		rspw.WriteHeader(http.StatusBadRequest)
		rspw.Write([]byte("'Sec-Websocket-Key' header missed"))
		return nil
	}
	if val := req.Header.Get("Sec-Websocket-Version"); val != "" {
		version, err := strconv.Atoi(val)
		if err != nil || version != 13 {
			rspw.WriteHeader(http.StatusBadRequest)
			rspw.Write([]byte("Invalid 'Sec-WebSocket-Version' header value (13 expected)"))
			return nil
		}
	} else {
		rspw.WriteHeader(http.StatusBadRequest)
		rspw.Write([]byte("'Sec-WebSocket-Version' header missed"))
		return nil
	}
	if val := req.Header.Get("Sec-Websocket-Extensions"); val != "" {
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
	handler := wsc.server.Config.Handshake(wsc, req, rspw)
	if handler != nil {
		rspw.Header().Set("Upgrade", "websocket")
		rspw.Header().Set("Connection", "Upgrade")
		rspw.Header().Set("Sec-WebSocket-Accept", acceptKey(req.Header.Get("Sec-Websocket-Key")))
		rspw.WriteHeader(http.StatusSwitchingProtocols)
	}
	return handler
}

//////////////// Read - Write interface ////////////////////

type MessageReader struct {
	wsc    *Connection
	frame  *Frame
	opened bool
	err    error
}

func (wsc *Connection) NewReader() *MessageReader {
	return &MessageReader{wsc: wsc}
}

func (mr *MessageReader) Read(b []byte) (int, error) {
	if mr.err != nil {
		return 0, mr.err
	}
	f := mr.frame
	if f == nil {
		f = newFrame(mr.wsc)
		if err := f.readHeader(); err != nil {
			mr.err = err
			return 0, mr.err
		}
		mr.wsc.LogDebug("frame header received: %s", f)
		switch f.Opcode {
		case OPCODE_PING, OPCODE_PONG, OPCODE_CLOSE:
			b, err := f.recv()
			if err != nil {
				mr.err = err
				return 0, mr.err
			}
			// control frame - return out of order as errors
			m := &Message{f.Opcode, b}
			if f.Opcode == OPCODE_CLOSE {
				mr.wsc.RcvdClose = m
				mr.err = io.EOF
			}
			return 0, m
		case OPCODE_TEXT, OPCODE_BINARY:
			if mr.opened {
				mr.err = ErrUnexpectedFrame
				return 0, mr.err
			} else {
				mr.opened = true
			}
		case OPCODE_CONTINUATION:
			if !mr.opened {
				mr.err = ErrUnexpectedContinuation
				return 0, mr.err
			}
		default:
			mr.err = ErrUnknownOpcode
			return 0, mr.err
		}
		mr.frame = f
	}
	n, err := f.read(b)
	mr.wsc.LogDebug("frame body read: %d", n)
	if err == EndOfFrame {
		if f.Fin {
			err = io.EOF
			mr.err = err
		} else {
			err = nil
			mr.frame = nil
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
		return 0, ErrMessageClosed
	}
	if mw.wsc.SentClose != nil || mw.wsc.closed {
		return 0, ErrConnectionClosed
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

func (wsc *Connection) Recv() (*Message, error) {
	if wsc.RcvdClose != nil || wsc.closed {
		return nil, io.EOF
	}
	for {
		f := newFrame(wsc)
		err := f.readHeader()
		if err != nil {
			return nil, err
		}
		wsc.LogDebug("frame header received: %s", f)

		if (f.Len > wsc.server.Config.MaxMsgLen) ||
			(f.Opcode == OPCODE_CONTINUATION && wsc.mm != nil && f.Len+wsc.mm.Len() > wsc.server.Config.MaxMsgLen) {
			wsc.mm = nil
			return nil, ErrMessageTooLarge
		}

		b, err := f.recv()
		if err != nil {
			return nil, err
		}
		wsc.LogDebug("frame body received: %d", len(b))

		switch f.Opcode {
		case OPCODE_BINARY, OPCODE_TEXT:
			if wsc.mm != nil {
				return nil, ErrUnexpectedFrame
			}
			if f.Fin {
				return &Message{f.Opcode, b}, nil
			} else {
				wsc.mm = &MultiframeMessage{f.Opcode, nil}
				wsc.mm.Append(b)
			}
		case OPCODE_CONTINUATION:
			if wsc.mm == nil {
				return nil, ErrUnexpectedContinuation
			}
			wsc.mm.Append(b)
			if f.Fin {
				m := wsc.mm.AsMessage()
				wsc.mm = nil
				return m, nil
			}
		case OPCODE_CLOSE:
			m := &Message{f.Opcode, b}
			wsc.RcvdClose = m
			return m, nil
		default:
			return &Message{f.Opcode, b}, nil
		}
	}
}

func (wsc *Connection) Send(msg *Message) error {
	if wsc.SentClose != nil || wsc.closed {
		return ErrConnectionClosed
	}
	f := newFrame(wsc)
	f.Len = len(msg.Body)
	f.Opcode = msg.Opcode
	if f.Opcode == OPCODE_PING || f.Opcode == OPCODE_PONG || f.Opcode == OPCODE_CLOSE {
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
	_, err = f.write(msg.Body)
	if err != nil {
		return err
	}
	wsc.LogDebug("frame body sent: %d", len(msg.Body))
	if msg.Opcode == OPCODE_CLOSE {
		wsc.SentClose = msg
	}
	return wsc.w.Flush()
}

func (wsc *Connection) SendText(b []byte) error {
	return wsc.Send(&Message{OPCODE_TEXT, b})
}

func (wsc *Connection) SendBinary(b []byte) error {
	return wsc.Send(&Message{OPCODE_BINARY, b})
}

func (wsc *Connection) SendPing(b []byte) error {
	return wsc.Send(&Message{OPCODE_PING, b})
}

func (wsc *Connection) SendPong(b []byte) error {
	return wsc.Send(&Message{OPCODE_PONG, b})
}

func (wsc *Connection) SendClose(code uint16, reason string) error {
	return wsc.Send(&Message{OPCODE_CLOSE, BuildCloseBody(code, reason)})
}

func (wsc *Connection) SendCloseError(err error) error {
	return wsc.Send(&Message{OPCODE_CLOSE, BuildCloseBodyError(err)})
}

func (wsc *Connection) Close() error {
	err := wsc.conn.Close()
	wsc.closed = true
	wsc.LogDebug("socket closed")
	return err
}

func (wsc *Connection) CloseGraceful(code uint16, reason string) error {
	if wsc.SentClose == nil {
		if wsc.RcvdClose == nil {
			_ = wsc.SendClose(code, reason)
		} else {
			_ = wsc.Send(wsc.RcvdClose)
		}
	}
	if wsc.RcvdClose == nil {
		wsc.SetReadDeadlineDuration(wsc.server.Config.CloseTimeout)
		for {
			msg, err := wsc.Recv()
			if err != nil || msg.Opcode == OPCODE_CLOSE {
				break
			}
		}
	}
	return wsc.Close()
}

func (wsc *Connection) CloseGracefulError(err error) error {
	return wsc.CloseGraceful(Err2CodeReason(err))
}

//////////////// Options ////////////////////

func (wsc *Connection) SetReadDeadline(t time.Time) error {
	return wsc.conn.SetReadDeadline(t)
}

func (wsc *Connection) SetReadDeadlineDuration(d time.Duration) error {
	var t time.Time
	if d > 0 {
		t = time.Now().Add(d)
	}
	return wsc.conn.SetReadDeadline(t)
}

func (wsc *Connection) SetWriteDeadline(t time.Time) error {
	return wsc.conn.SetWriteDeadline(t)
}

func (wsc *Connection) SetWriteDeadlineDuration(d time.Duration) error {
	var t time.Time
	if d > 0 {
		t = time.Now().Add(d)
	}
	return wsc.conn.SetWriteDeadline(t)
}

//////////////// Logging ////////////////////

const (
	LOG_ERROR = 0
	LOG_WARN  = 1
	LOG_INFO  = 2
	LOG_DEBUG = 3
)

var logNames map[uint8]string = map[uint8]string{
	LOG_ERROR: "ERROR",
	LOG_WARN:  "WARN",
	LOG_INFO:  "INFO",
	LOG_DEBUG: "DEBUG",
}

func (wsc *Connection) Log(level uint8, format string, args ...interface{}) {
	if level > wsc.LogLevel {
		return
	}
	addr := wsc.conn.RemoteAddr().String()
	msg := fmt.Sprintf("%s %s: ", addr, logNames[level]) + fmt.Sprintf(format, args...)
	log.Println(msg)
}

func (wsc *Connection) LogError(fmt string, args ...interface{}) {
	wsc.Log(LOG_ERROR, fmt, args...)
}

func (wsc *Connection) LogWarn(fmt string, args ...interface{}) {
	wsc.Log(LOG_WARN, fmt, args...)
}

func (wsc *Connection) LogInfo(fmt string, args ...interface{}) {
	wsc.Log(LOG_INFO, fmt, args...)
}

func (wsc *Connection) LogDebug(fmt string, args ...interface{}) {
	wsc.Log(LOG_DEBUG, fmt, args...)
}
