# wsd
WebSocket daemon in Go. Trying to optimize memory and CPU consumption

# example

    package main

    import (
        "io"
        "log"
    )

    func echoHandler(wsc *websocket.Connection) {
        for {
            var err error
            msg, err := wsc.Recv()
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
            err = wsc.SendText(msg)
            if err == io.EOF {
                return
            }
        }
    }

    func echoHandshake(req *websocket.HttpRequest, rsp *websocket.HttpResponse) websocket.HandlerFunc {
        return echoHandler
    }

    func main() {
        server := websocket.Server{
            Addr:      ":8090",
            Handshake: echoHandshake,
        }
        log.Fatal(server.Serve())
    }
