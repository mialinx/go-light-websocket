package websocket

import (
	"bufio"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrRequestLineTooLong = errors.New("Request line is out of buffer")
	ErrHeaderLineTooLong  = errors.New("Header line is out of buffer")
	ErrInvalidSyntax      = errors.New("Invalid request syntax")
)

type HttpRequest struct {
	Method  string
	Version string
	Uri     string
	Headers map[string]string
	form    map[string]string
}

type HttpResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       string
}

func newHttpRequest() *HttpRequest {
	req := &HttpRequest{}
	req.Headers = make(map[string]string, 8)
	return req
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadSlice('\n')
	if err == bufio.ErrBufferFull {
		return "", ErrRequestLineTooLong
	} else if err != nil {
		return "", err
	}
	line = line[0 : len(line)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[0 : len(line)-1]
	}
	return string(line), nil
}

func normalizeHeader(h string) string {
	n := []byte(h)
	shift := byte('a') - byte('A')
	first := true
	for i := 0; i < len(n); i++ {
		if first {
			if n[i] >= 'a' && n[i] <= 'z' {
				n[i] -= shift
			}
		} else {
			if n[i] >= 'A' && n[i] <= 'Z' {
				n[i] += shift
			}
		}
		if n[i] == '-' {
			first = true
		} else {
			first = false
		}
	}
	return string(n)
}

func (req *HttpRequest) ReadFrom(r *bufio.Reader) error {
	line, err := readLine(r)
	if err != nil {
		return err
	}
	parts := strings.Split(line, " ")
	if len(parts) != 3 {
		return ErrInvalidSyntax
	}
	req.Method = strings.ToUpper(parts[0])
	req.Uri = parts[1]
	req.Version = strings.ToUpper(parts[2])
	var hn string
	const lws = "\t "
	for {
		line, err := readLine(r)
		if err != nil {
			return err
		}
		if line == "" {
			break
		}
		// multiline header
		if (line[0] == ' ' || line[0] == '\t') && hn != "" {
			req.Headers[hn] += " " + strings.Trim(line, lws)
			continue
		}
		if i := strings.IndexByte(line, ':'); i > 0 {
			hn = normalizeHeader(line[0:i])
			if _, ok := req.Headers[hn]; ok {
				// multiple Headers with same name
				req.Headers[hn] += ", " + strings.Trim(line[i+1:], lws)
			} else {
				req.Headers[hn] = strings.Trim(line[i+1:], lws)
			}
			continue
		}
		return ErrInvalidSyntax
	}
	return nil
}

func (req *HttpRequest) FormValue(k string) string {
	if req.form == nil {
		req.form = make(map[string]string, 5)
		// parse form light
		if li := strings.LastIndex(req.Uri, "?"); li > -1 {
			qs := req.Uri[li+1:]
			for _, pair := range strings.Split(qs, "&") {
				kv := strings.SplitN(pair, "=", 2)
				k, _ := url.QueryUnescape(kv[0])
				v, _ := url.QueryUnescape(kv[1])
				req.form[k] = v
			}
		}
	}
	return req.form[k]
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
