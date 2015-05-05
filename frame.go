package websocket

import (
	"bufio"
	"fmt"
	"io"
)

type Frame struct {
	Fin    bool
	Opcode byte
	Mask   bool
	Key    [4]byte
	Len    int
	r      *bufio.Reader
	w      *bufio.Writer
	stats  *Stats
	done   int
}

func (f *Frame) String() string {
	return fmt.Sprintf("Frame{ Opcode: %d, Fin: %v, Len: %d, done: %d, Mask: %v, Key: [% x] }", f.Opcode, f.Fin, f.Len, f.done, f.Mask, f.Key)
}

func newFrame(wsc *Connection) *Frame {
	return &Frame{
		r:     wsc.r,
		w:     wsc.w,
		stats: wsc.server.Stats,
	}
}

func (f *Frame) readHeader() error {
	var b [8]byte
	if _, err := io.ReadFull(f.r, b[0:2]); err != nil {
		return err
	}
	f.Fin = (b[0] & 0x80) > 0
	f.Opcode = b[0] & 0x0f
	f.Mask = (b[1] & 0x80) > 0
	f.Len = int(b[1]) & 0x7f
	if f.Len == 126 {
		if _, err := io.ReadFull(f.r, b[0:2]); err != nil {
			return err
		}
		f.Len = 0
		f.Len += int(b[0]) << 8
		f.Len += int(b[1])
	} else if f.Len == 127 {
		if _, err := io.ReadFull(f.r, b[:]); err != nil {
			return err
		}
		f.Len = 0
		f.Len += int(b[0]) << 56
		f.Len += int(b[1]) << 48
		f.Len += int(b[2]) << 40
		f.Len += int(b[3]) << 32
		f.Len += int(b[4]) << 24
		f.Len += int(b[5]) << 16
		f.Len += int(b[6]) << 8
		f.Len += int(b[7])
		if f.Len < 0 {
			return ErrMsgTooLong
		}
	}
	if f.Mask {
		if _, err := io.ReadFull(f.r, f.Key[:]); err != nil {
			return err
		}
	}
	f.stats.add(eventInFrame{})
	return nil
}

func (f *Frame) writeHeader() error {
	b := make([]byte, 10)
	if f.Fin {
		b[0] |= 0x80
	}
	b[0] |= (f.Opcode & 0x0f)
	if f.Len < 126 {
		b[1] = byte(f.Len & 0x7f)
		b = b[0:2]
	} else if f.Len <= 0xFFFF {
		b[1] = 126
		b[2] = byte((f.Len >> 8) & 0xFF)
		b[3] = byte(f.Len & 0xFF)
		b = b[0:4]
	} else {
		b[1] = 127
		b[2] = byte((f.Len >> 56) & 0xFF)
		b[3] = byte((f.Len >> 48) & 0xFF)
		b[4] = byte((f.Len >> 40) & 0xFF)
		b[5] = byte((f.Len >> 32) & 0xFF)
		b[6] = byte((f.Len >> 24) & 0xFF)
		b[7] = byte((f.Len >> 16) & 0xFF)
		b[8] = byte((f.Len >> 8) & 0xFF)
		b[9] = byte(f.Len & 0xFF)
	}
	for len(b) > 0 {
		n, err := f.w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	f.stats.add(eventOutFrame{})
	return nil
}

func (f *Frame) read(b []byte) (int, error) {
	if f.done >= f.Len {
		return 0, EndOfFrame
	}
	if need := f.Len - f.done; len(b) > need {
		b = b[:need]
	}
	n, err := f.r.Read(b)
	for i := 0; i < n; i++ {
		b[i] ^= f.Key[(f.done+i)%4]
	}
	f.done += n
	return n, err
}

func (f *Frame) write(b []byte) (int, error) {
	if len(b) > f.Len-f.done {
		panic(fmt.Sprintf("writing more than fame len. f.Len=%d f.done=%d len(b)=%d", f.done, f.Len, len(b)))
	}
	n, err := f.w.Write(b)
	f.done += n
	return n, err
}

func (f *Frame) recv() ([]byte, error) {
	b := make([]byte, f.Len)
	for f.done < f.Len {
		_, err := f.read(b[f.done:])
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

func (f *Frame) send(b []byte) error {
	if f.Len != len(b) {
		panic("buffer len does not match fram len")
	}
	_, err := f.write(b[f.done:])
	return err
}
