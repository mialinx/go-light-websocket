package main

import (
	"io"
	"log"
	"wsd/ws"
)

func echoHandler(wsc *ws.Connection) {
	for {
		var err error
		msg, err := wsc.Recv()
		log.Printf("msg1: %d err: %s", len(msg), err)
		if string(msg) == "enough" {
			wsc.Close()
			return
		}
		if err == io.EOF {
			return
		}
		l := len(msg)
		for i := 0; i < l/2; i++ {
			msg[i], msg[l-i-1] = msg[l-i-1], msg[i]
		}
		//log.Printf("msg2: %s", msg)
		err = wsc.SendText(msg)
		if err == io.EOF {
			return
		}
	}
}

func echoHandshake(req *ws.HttpRequest, rsp *ws.HttpResponse) ws.HandlerFunc {
	return echoHandler
}

func main() {
	server := ws.Server{
		Addr:      ":8090",
		Handshake: echoHandshake,
	}
	log.Fatal(server.Serve())
}
