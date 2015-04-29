package websocket

import (
	"log"
	"net"
	"runtime/debug"
	"time"
)

const (
	AcceptErrorTimeout = time.Second
)

type Server struct {
	Addr            string
	Handshake       HandshakeFunc
	MaxMsgLen       int
	ReadBufferSize  int
	WriteBufferSize int
}

func (s *Server) serve(conn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("ERROR: %s\n%s", err, debug.Stack())
		}
	}()
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
		go s.serve(conn)
	}
}
