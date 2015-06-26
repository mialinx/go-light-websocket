package websocket

import (
	"crypto/tls"
	"log"
	"net"
	"time"
)

type Server struct {
	Config *Config
	Stats  *Stats
}

type Config struct {
	Handshake             HandshakeFunc
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
	if config.SockReadBuffer == 0 {
		config.SockReadBuffer = DefaultSockReadBuffer
	}
	if config.SockWriteBuffer == 0 {
		config.SockWriteBuffer = DefaultSockReadBuffer
	}
	if config.HttpReadBuffer == 0 {
		config.HttpReadBuffer = DefaultHttpReadBuffer
	}
	if config.HttpWriteBuffer == 0 {
		config.HttpWriteBuffer = DefaultHttpReadBuffer
	}
	if config.WsReadBuffer == 0 {
		config.WsReadBuffer = DefaultWsReadBuffer
	}
	if config.WsWriteBuffer == 0 {
		config.WsWriteBuffer = DefaultWsReadBuffer
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

func (s *Server) Serve(addr string) (err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.serve(ln)
	return
}

func (s *Server) ServeTLS(addr string, certFile string, keyFile string) (err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	config := new(tls.Config)
	config.NextProtos = []string{"http/1.1"}
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	tlsLn := tls.NewListener(ln, config)
	s.serve(tlsLn)
	return
}
