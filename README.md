# wsd
WebSocket daemon in Go. Trying to optimize memory and CPU consumption.

# example

    package main

    import (
        "github.com/mialinx/go-light-websocket"
        "log"
        "net/http"
    )

    func handshake(wsc *websocket.Connection, req *http.Request, rspw http.ResponseWriter) websocket.HandlerFunc {
        reverse := false
        if r := req.FormValue("reverse"); r == "1" {
            reverse = true
        }
        return func(wsc *websocket.Connection) error {
            return handler(wsc, reverse)
        }
    }

    func handler(wsc *websocket.Connection, reverse bool) error {
        for {
            msg, err := wsc.Recv()
            if err != nil {
                return err
            }
            switch msg.Opcode {
            case websocket.OPCODE_PING:
                msg.Opcode = websocket.OPCODE_PONG
                err := wsc.Send(msg)
                if err != nil {
                    return err
                }
            case websocket.OPCODE_PONG:
                // okay, pong
            case websocket.OPCODE_TEXT, websocket.OPCODE_BINARY:
                if string(msg.Body) == "exit" {
                    wsc.CloseGraceful(nil)
                    break
                }
                if reverse {
                    l : = len(msg.Body)
                    for i := 0; i < l/2; i++ {
                        msg.Body[i], msg.Body[l-i-1] = msg.Body[l-i-1], msg.Body[i]
                    }
                }
                err := wsc.Send(msg)
                if err != nil {
                    return err
                }
            }
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
