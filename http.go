package websocket

import (
	"bytes"
	"io"
	"net/http"
)

type httpBodyBuffer struct {
	bytes.Buffer
}

func (hbb *httpBodyBuffer) Close() error {
	return nil
}

type httpResponseWriter struct {
	rsp  http.Response
	body httpBodyBuffer
}

func newHtttpResponseWriter() *httpResponseWriter {
	hrw := new(httpResponseWriter)
	hrw.rsp.ProtoMajor = 1
	hrw.rsp.ProtoMinor = 1
	hrw.rsp.Header = make(http.Header)
	hrw.rsp.Body = &hrw.body
	return hrw
}

func (hrw *httpResponseWriter) Header() http.Header {
	return hrw.rsp.Header
}

func (hrw *httpResponseWriter) WriteHeader(status int) {
	hrw.rsp.StatusCode = status
}

func (hrw *httpResponseWriter) Write(b []byte) (int, error) {
	return hrw.body.Write(b)
}

func (hrw *httpResponseWriter) WriteTo(w io.Writer) error {
	hrw.rsp.ContentLength = int64(hrw.body.Len())
	return hrw.rsp.Write(w)
}
