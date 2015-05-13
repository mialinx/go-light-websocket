package websocket

import (
	"time"
)

type ChannelHandler func(rc <-chan *Message, wc chan<- *Message) error

func reader(wsc *Connection, rc chan *Message, wc chan *Message) {
	for {
		msg, err := wsc.Recv()
		if err != nil {
			//wsc.LogError(err.Error())
			wsc.LogError("1) %T %v %s", err, err, err.Error())
			wsc.LogInfo("closing rc")
			close(rc)
			wc <- &Message{OPCODE_CLOSE, Err2Close(err)}
			return
		}
		switch msg.Opcode {
		case OPCODE_PING:
			wc <- &Message{OPCODE_PONG, msg.Body}
		case OPCODE_PONG:
			// okay, ignore it
		case OPCODE_CLOSE:
			// echo close frame back to client
			close(rc)
			wc <- msg
			return
		default:
			// pass to handler
			rc <- msg
		}
	}
}

func writer(wsc *Connection, rc chan *Message, wc chan *Message) {
	for {
		msg, ok := <-wc
		if !ok {
			wsc.LogError("wc unexpectedly closed")
			wsc.Close()
			return
		}
		err := wsc.Send(msg)
		if err != nil {
			wsc.LogError(err.Error())
			break
		}
		if msg.Opcode == OPCODE_CLOSE {
			break
		}
	}

	serverClose := wsc.RcvdClose == nil
	t := time.NewTimer(wsc.server.Config.CloseTimeout)
OUT:
	for {
		select {
		case msg := <-wc:
			if serverClose {
				if wsc.RcvdClose != nil {
					break OUT
				}
			} else {
				if msg == nil || msg.Opcode == OPCODE_CLOSE {
					break OUT
				}
			}
		case <-t.C:
			wsc.LogWarn("timeout while closing connection (serverClose=%s)", serverClose)
			break OUT
		}
	}
	wsc.Close()
	return
}

func WrapChannelHandler(handler ChannelHandler, l int) HandlerFunc {
	return func(wsc *Connection) error {
		rc := make(chan *Message, l)
		wc := make(chan *Message, l)
		go reader(wsc, rc, wc)
		go writer(wsc, rc, wc)
		err := handler(rc, wc)
		wc <- &Message{OPCODE_CLOSE, Err2Close(err)}
		return err
	}
}
