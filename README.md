# go-light-websocket

WebSocket protocol implementation with minimal memory overhead.

# example

```golang
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
                wsc.CloseGracefulError(nil)
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
    server := websocket.NewServer(websocket.Config{
        Addr:            ":443",
        CertFile:        "/path/to/cert.crt",
        KeyFile:         "/path/to/cert.key",
        Handshake:       handshake,
        MaxMsgLen:       16 * 1024 * 1024,
        SockReadBuffer:  4 * 1024 * 1024,
        SockWriteBuffer: 4 * 1024 * 1024,
        IOStatistics:    true,
        LogLevel:        websocket.LOG_INFO,
    })
    go func() {
        log.Fatalln(server.ServeTLS())
    }()
}
```

# options

#### MaxMsgLen             int
Maximal length of message, that can be buffered into memory

#### SockReadBuffer        int
#### SockWriteBuffer       int
TCP read/write buffer sizes (in kernel)
In case you don't read much data from socket - you may set minimal size

#### HttpReadBuffer        int
#### HttpWriteBuffer       int
Userspace buffer for parsing http request and sending http response.
Http requests may containg long headers, you may need large enough buffers for handshake.

#### WsReadBuffer          int
#### WsWriteBuffer         int
Userspace buffer for websocket framing protocol.
Websocket is simple protocol, you may set the minimal value - 16 byte to avoid 
userspace buffering. It will increase number of syscalls, but saves some memory.

#### IOStatistics          bool
Enables IO statistics - number of currently reading and writing connections.

#### LogLevel              uint8
Server log level. With DEBUG will print all sent and received frames.

#### HandshakeReadTimeout  time.Duration
#### HandshakeWriteTimeout time.Duration
Timeouts to read http request and send http response (handshake).

#### CloseTimeout          time.Duration
Timeout to wait for websocket Close (ack) frame.

#### TCPKeepAlive          time.Duration
Enables TCP KeepAlive if not zero.


# faq 

#### why not use standard http server ?
Standard http server but doesn't allow fine-tuning of connections and buffers.
So it's consumes more memory and decrease maximal number of connections per server.
