package main

import (
	"fmt"
	"github.com/mialinx/go-light-websocket"
	"log"
	"net/http"
	"strconv"
	"time"
)

func handshake(wsc *websocket.Connection, req *http.Request, rspw http.ResponseWriter) websocket.HandlerFunc {
	return websocket.WrapChannelHandler(handler, 1)
}

func handler(rc <-chan *websocket.Message, wc chan<- *websocket.Message) error {
	for msg := range rc {
		n, err := strconv.Atoi(string(msg.Body))
		if err != nil {
			wc <- &websocket.Message{websocket.OPCODE_TEXT, []byte(err.Error())}
			continue
		}
		if n == 0 {
			break
		}
		for ; n > 0; n-- {
			time.Sleep(time.Second)
			wc <- &websocket.Message{websocket.OPCODE_TEXT, []byte(fmt.Sprintf("%d...", n))}
		}
		wc <- &websocket.Message{websocket.OPCODE_TEXT, []byte("boom!")}
	}
	return nil
}

func main() {
	server := websocket.NewServer(websocket.Config{
		Addr:            ":1234",
		Handshake:       handshake,
		MaxMsgLen:       16 * 1024 * 1024,
		SockReadBuffer:  4 * 1024 * 1024,
		SockWriteBuffer: 4 * 1024 * 1024,
		IOStatistics:    true,
		LogLevel:        websocket.LOG_INFO,
	})
	log.Fatalln(server.Serve())
}
