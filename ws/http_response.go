package ws

import (
	"bufio"
	"net/http"
	"strconv"
)

type HttpResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       string
}

func newHttpResponse() *HttpResponse {
	rsp := &HttpResponse{}
	rsp.Headers = make(map[string]string)
	return rsp
}

func (rsp *HttpResponse) WriteTo(w *bufio.Writer) {
	var status_text string
	if rsp.StatusText != "" {
		status_text = rsp.StatusText
	} else {
		status_text = http.StatusText(rsp.Status)
	}
	w.WriteString("HTTP/1.1 ")
	w.WriteString(strconv.Itoa(rsp.Status))
	w.WriteString(" ")
	w.WriteString(status_text)
	w.WriteString("\r\n")
	if rsp.Body != "" {
		rsp.Headers["Content-Length"] = strconv.Itoa(len(rsp.Body))
	}
	for h, v := range rsp.Headers {
		w.WriteString(h)
		w.WriteString(": ")
		w.WriteString(v)
		w.WriteString("\r\n")
	}
	w.WriteString("\r\n")
	w.WriteString(rsp.Body)
	w.Flush()
}
