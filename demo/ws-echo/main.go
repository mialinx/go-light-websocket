package main

import (
	"github.com/mialinx/go-light-websocket"
	"log"
	"net/http"
)

func handshake(wsc *websocket.Connection, req *http.Request, rspw http.ResponseWriter) websocket.HandlerFunc {
	return websocket.WrapChannelHandler(handler, 1)
}

func handler(rc <-chan *websocket.Message, wc chan<- *websocket.Message) error {
	for msg := range rc {
		wc <- msg
	}
	return nil
}

func main() {
	server := websocket.NewServer(":1234", handshake, websocket.Config{
		MaxMsgLen:       16 * 1024 * 1024,
		SockReadBuffer:  4 * 1024 * 1024,
		SockWriteBuffer: 4 * 1024 * 1024,
		IOStatistics:    true,
		LogLevel:        websocket.LOG_INFO,
	})
	log.Fatalln(server.Serve())
}
