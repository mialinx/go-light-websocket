package websocket

import (
	"fmt"
	"log"
	"time"
)

type RpsCounter struct {
	Count uint64
	buf   []int64
	i     int
}

func newEvStat() *RpsCounter {
	rc := &RpsCounter{}
	rc.buf = make([]int64, 1000)
	return rc
}

func (rc *RpsCounter) inc() {
	rc.Count++
	ts := time.Now().UnixNano()
	rc.buf[rc.i] = ts
	rc.i = (rc.i + 1) % len(rc.buf)
}

func (rc *RpsCounter) Rps() float64 {
	n := len(rc.buf)
	j := (rc.i + 1) % n
	tsMax := rc.buf[rc.i]
	tsMin := rc.buf[j]
	if tsMin == 0 {
		tsMin = rc.buf[0]
		n = rc.i + 1
	}
	return float64(n*1e9) / float64(tsMax-tsMin)
}

func (rc *RpsCounter) String() string {
	return fmt.Sprintf("%d (%2f)", rc.Count, rc.Rps())
}

var knownOpcodes []uint8 = []uint8{
	OPCODE_CONTINUATION,
	OPCODE_TEXT,
	OPCODE_BINARY,
	OPCODE_CLOSE,
	OPCODE_PING,
	OPCODE_PONG,
}

type Stats struct {
	Connections        uint64
	ConnectionsReading uint64
	ConnectionsWriting uint64
	Handshakes         *RpsCounter
	HandshakesFailed   *RpsCounter
	InFrames           map[uint8]*RpsCounter
	OutFrames          map[uint8]*RpsCounter
	channel            chan interface{}
}

func (st *Stats) String() string {
	s := ""
	s += fmt.Sprintf("Connections: %d\n", st.Connections)
	s += fmt.Sprintf("\tReading: %d\n", st.ConnectionsReading)
	s += fmt.Sprintf("\tWriring: %d\n", st.ConnectionsWriting)
	s += fmt.Sprintf("Handshakes: %s\n", st.Handshakes)
	s += fmt.Sprintf("HandshakesFailed: %s\n", st.HandshakesFailed)
	s += "InFrames\n"
	for _, opcode := range knownOpcodes {
		s += fmt.Sprintf("\t%d: %s", opcode, st.InFrames[opcode])
	}
	s += "OutFrames\n"
	for _, opcode := range knownOpcodes {
		s += fmt.Sprintf("\t%d: %s", opcode, st.OutFrames[opcode])
	}
	return s
}

func newStats() *Stats {
	s := &Stats{}
	s.Handshakes = newEvStat()
	s.HandshakesFailed = newEvStat()
	s.InFrames = make(map[uint8]*RpsCounter, 10)
	s.OutFrames = make(map[uint8]*RpsCounter, 10)
	for _, opcode := range knownOpcodes {
		s.InFrames[opcode] = newEvStat()
		s.OutFrames[opcode] = newEvStat()
	}
	s.channel = make(chan interface{}, 1024)
	go s.handler()
	return s
}

type eventConnect struct{}
type eventClose struct{ d time.Duration }
type eventHandshake struct{}
type eventHandshakeFailed struct{}
type eventReadStart struct{}
type eventReadStop struct{ d time.Duration }
type eventWriteStart struct{}
type eventWriteStop struct{ d time.Duration }
type eventInFrame struct{ opcode uint8 }
type eventOutFrame struct{ opcode uint8 }

func (st *Stats) add(v interface{}) {
	st.channel <- v
	//select {
	//case st.channel <- v:
	//	return
	//default:
	//	return
	//}
}

func (st *Stats) handler() {
	for ev := range st.channel {
		switch ev := ev.(type) {
		case eventConnect:
			st.Connections++
		case eventClose:
			if st.Connections > 0 {
				st.Connections--
			} else {
				log.Printf("stats: Connections below zero")
			}
		case eventHandshake:
			st.Handshakes.inc()
		case eventHandshakeFailed:
			st.HandshakesFailed.inc()
		case eventReadStart:
			st.ConnectionsReading++
		case eventReadStop:
			if st.ConnectionsReading > 0 {
				st.ConnectionsReading--
			} else {
				log.Printf("stats: ConnectionsReading below zero")
			}
		case eventWriteStart:
			st.ConnectionsWriting++
		case eventWriteStop:
			if st.ConnectionsWriting > 0 {
				st.ConnectionsWriting--
			} else {
				log.Printf("stats: ConnectionsWriting below zero")
			}
		case eventInFrame:
			if fs, ok := st.InFrames[ev.opcode]; ok {
				fs.inc()
			} else {
				log.Printf("stats: unknown opcode %d", ev.opcode)
			}
		case eventOutFrame:
			if fs, ok := st.InFrames[ev.opcode]; ok {
				fs.inc()
			} else {
				log.Printf("stats: unknown opcode %d", ev.opcode)
			}
		default:
			panic(fmt.Sprintf("unknown stat event type %v", ev))
		}
	}
}
