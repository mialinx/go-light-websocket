package websocket

import (
	"crypto/tls"
	"log"
	"net"
	"strings"
	"time"
)

type Server struct {
	Config *Config
	Stats  *Stats
}

type Config struct {
	Handshake             HandshakeFunc
	Addr                  string
	CertFile              string
	KeyFile               string
	MaxMsgLen             int
	SockReadBuffer        int
	SockWriteBuffer       int
	HttpReadBuffer        int
	HttpWriteBuffer       int
	WsReadBuffer          int
	WsWriteBuffer         int
	IOStatistics          bool
	LogLevel              uint8
	CloseTimeout          time.Duration
	HandshakeReadTimeout  time.Duration
	HandshakeWriteTimeout time.Duration
	TCPKeepAlive          time.Duration
}

func NewServer(config Config) *Server {
	if config.Handshake == nil {
		panic("config.Handshake is not set")
	}
	if config.Addr == "" {
		panic("config.Addr is not set")
	}
	if config.SockReadBuffer == 0 {
		config.SockReadBuffer = DefaultSockReadBuffer
	}
	if config.SockWriteBuffer == 0 {
		config.SockWriteBuffer = DefaultSockWriteBuffer
	}
	if config.HttpReadBuffer == 0 {
		config.HttpReadBuffer = DefaultHttpReadBuffer
	}
	if config.HttpWriteBuffer == 0 {
		config.HttpWriteBuffer = DefaultHttpWriteBuffer
	}
	if config.WsReadBuffer == 0 {
		config.WsReadBuffer = DefaultWsReadBuffer
	}
	if config.WsWriteBuffer == 0 {
		config.WsWriteBuffer = DefaultWsWriteBuffer
	}
	if config.MaxMsgLen == 0 {
		config.MaxMsgLen = DefaultMaxMsgLen
	}
	if config.HandshakeReadTimeout == 0 {
		config.HandshakeReadTimeout = DefaultHandshakeReadTimeout
	}
	if config.HandshakeWriteTimeout == 0 {
		config.HandshakeWriteTimeout = DefaultHandshakeWriteTimeout
	}
	if config.CloseTimeout == 0 {
		config.CloseTimeout = DefaultCloseTimeout
	}
	s := &Server{
		Config: &config,
		Stats:  newStats(),
	}
	return s
}

func (s *Server) serve(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("ERROR: Failed to accept connection %s", err)
			time.Sleep(AcceptErrorTimeout)
			continue
		}
		go func() {
			wsc := newConnection(s, conn)
			wsc.serve()
		}()
	}
}

func (s *Server) Serve() (err error) {
	ln, err := net.Listen("tcp", s.Config.Addr)
	if err != nil {
		return err
	}
	s.serve(ln)
	return
}

func (s *Server) ServeTLS() (err error) {
	ln, err := net.Listen("tcp", s.Config.Addr)
	if err != nil {
		return err
	}
	if s.Config.CertFile == "" || s.Config.KeyFile == "" {
		panic("cert-file or key-file not specified")
	}
	config := new(tls.Config)
	certs := strings.Split(s.Config.CertFile, ",")
	keyfiles := strings.Split(s.Config.KeyFile, ",")
	minLen := len(certs)
	if len(keyfiles) < minLen {
		minLen = len(keyfiles)
	}
	config.Certificates = make([]tls.Certificate, minLen)
	for i := 0; i < minLen; i++ {
		config.Certificates[i], err = tls.LoadX509KeyPair(certs[i], keyfiles[i])
	}

	config.NextProtos = []string{"http/1.1"}
	// select only strong ciphers from this list https://golang.org/pkg/crypto/tls/#pkg-constants
	config.CipherSuites = []uint16{0x002f, 0x0035, 0x003c, 0x009c, 0x009d, 0xc007, 0xc009, 0xc00a, 0xc013, 0xc014,
		0xc023, 0xc027, 0xc02f, 0xc02b, 0xc030, 0xc02c, 0xcca8, 0xcca9, 0x1301, 0x1302, 0x1303}
	if err != nil {
		return err
	}
	tlsLn := tls.NewListener(ln, config)
	s.serve(tlsLn)
	return
}
