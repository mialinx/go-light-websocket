package main

import (
	"log"
	"wsd/ws"
)

func echoHandler(wsc *ws.Connection) {
	for {
		msg, _ := wsc.Recv()
		log.Printf("msg1: %s", msg)
		if string(msg) == "enough" {
			wsc.Close()
			return
		}
		l := len(msg)
		for i := 0; i < l/2; i++ {
			msg[i], msg[l-i] = msg[l-i], msg[i]
		}
		log.Printf("msg2: %s", msg)
		wsc.Send(msg)
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
