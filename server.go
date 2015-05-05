package websocket

import (
	"log"
	"net"
	"runtime/debug"
	"time"
)

type Server struct {
	Addr      string
	Handshake HandshakeFunc
	Config    *Config
	Stats     *Stats
}

type Config struct {
	MaxMsgLen       int
	ReadBufferSize  int
	WriteBufferSize int
	IOStatistics    bool
}

func NewServer(addr string, handshake HandshakeFunc, config Config) *Server {
	if config.ReadBufferSize == 0 {
		config.ReadBufferSize = DefaultReadBufferSize
	}
	if config.WriteBufferSize == 0 {
		config.WriteBufferSize = DefaultReadBufferSize
	}
	if config.MaxMsgLen == 0 {
		config.MaxMsgLen = DefaultMaxMsgLen
	}
	s := &Server{
		Addr:      addr,
		Handshake: handshake,
		Config:    &config,
		Stats:     newStats(),
	}
	return s
}

func (s *Server) serveConnection(conn net.Conn) {
	defer func() {
		s.Stats.add(eventClose{})
		if err := recover(); err != nil {
			log.Printf("ERROR: %s\n%s", err, debug.Stack())
		}
	}()
	s.Stats.add(eventConnect{})
	wsc := newConnection(s, conn)
	wsc.serve()
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
			log.Printf("Failed to accept connection %s", err)
			time.Sleep(AcceptErrorTimeout)
			continue
		}
		go s.serveConnection(conn)
	}
}
