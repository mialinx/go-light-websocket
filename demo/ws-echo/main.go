package main

import (
	"github.com/mialinx/go-light-websocket"
	"log"
)

func handshake(req *websocket.HttpRequest, rsp *websocket.HttpResponse) websocket.HandlerFunc {
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
		ReadBufferSize:  4 * 1024 * 1024,
		WriteBufferSize: 4 * 1024 * 1024,
		IOStatistics:    true,
		LogLevel:        websocket.LOG_INFO,
	})
	log.Fatalln(server.Serve())
}
