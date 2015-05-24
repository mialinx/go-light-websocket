package websocket

import (
	"log"
	"net"
	"time"
)

type Server struct {
	Addr      string
	Handshake HandshakeFunc
	Config    *Config
	Stats     *Stats
}

type Config struct {
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
}

func NewServer(addr string, handshake HandshakeFunc, config Config) *Server {
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
		Addr:      addr,
		Handshake: handshake,
		Config:    &config,
		Stats:     newStats(),
	}
	return s
}

func (s *Server) Serve() (err error) {
	if s.Handshake == nil {
		panic("Hanshake is nil")
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("ERROR: Failed to accept connection %s", err)
			time.Sleep(AcceptErrorTimeout)
			continue
		}
		go func() {
			wsc := newConnection(s, conn.(*net.TCPConn))
			wsc.serve()
		}()
	}
}
