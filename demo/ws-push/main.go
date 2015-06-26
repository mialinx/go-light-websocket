package main

import (
	"github.com/mialinx/go-light-websocket"
	"io"
	"log"
	"net/http"
	"sync"
)

var registry map[string]chan *websocket.Message = make(map[string]chan *websocket.Message)
var regMutex sync.Mutex

const tubeLen = 4

func reader(wsc *websocket.Connection, wc chan *websocket.Message) {
	defer close(wc)
	for {
		msg, err := wsc.Recv()
		if err != nil {
			if err != io.EOF {
				wsc.LogError("reader: ", err.Error())
			}
			return
		}
		switch msg.Opcode {
		case websocket.OPCODE_PING:
			msg.Opcode = websocket.OPCODE_PONG
			wc <- msg
		case websocket.OPCODE_CLOSE:
			wc <- msg
			return
		}
	}
}

func writer(wsc *websocket.Connection, wc, t chan *websocket.Message) {
	defer wsc.Close()
	for {
		select {
		case msg := <-t:
			err := wsc.Send(msg)
			if err != nil {
				wsc.LogError("writer: ", err.Error())
				return
			}
		case msg, ok := <-wc:
			if !ok {
				wsc.LogDebug("writer: reader closed signal")
				return
			}
			err := wsc.Send(msg)
			if err != nil {
				wsc.LogError("writer: ", err.Error())
				return
			}
			if msg.Opcode == websocket.OPCODE_CLOSE {
				return
			}
		}
	}
	return
}

func handler(wsc *websocket.Connection, id string) error {
	regMutex.Lock()
	t, ok := registry[id]
	if !ok {
		t = make(chan *websocket.Message, tubeLen)
		registry[id] = t
	}
	regMutex.Unlock()

	wc := make(chan *websocket.Message)
	go reader(wsc, wc)
	writer(wsc, wc, t)

	regMutex.Lock()
	delete(registry, id)
	regMutex.Unlock()

	return nil
}

func handshake(wsc *websocket.Connection, req *http.Request, rspw http.ResponseWriter) websocket.HandlerFunc {
	id := req.FormValue("id")
	if id == "" {
		rspw.WriteHeader(http.StatusBadRequest)
		rspw.Write([]byte("no_id"))
		return nil
	}
	return func(wsc *websocket.Connection) error {
		return handler(wsc, id)
	}
}

func respond(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(msg))
	return
}

func put(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id == "" {
		respond(w, 400, "no_id")
		return
	}
	msg := r.FormValue("msg")

	regMutex.Lock()
	t := registry[id]
	regMutex.Unlock()

	if t != nil {
		t <- &websocket.Message{websocket.OPCODE_TEXT, []byte(msg)}
		respond(w, 200, "ok")
	} else {
		respond(w, 200, "offline")
	}
	return
}

func main() {
	// run websocket server
	var wsServer = websocket.NewServer(websocket.Config{
		Handshake: handshake,
		LogLevel:  websocket.LOG_DEBUG,
	})
	go func() {
		log.Fatalln(wsServer.Serve(":1234"))
	}()

	// run internal interface
	http.HandleFunc("/put/", put)
	err := http.ListenAndServe("127.0.0.1:8080", nil)
	if err != nil {
		log.Fatalln(err)
	}
}
