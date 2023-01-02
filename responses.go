package myproxy

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

const (
	ContentTypeText = "text/plain"
	ContentTypeHtml = "text/html"
)

func NewResponse(r *http.Request, contentType string, status int, body string) *http.Response {
	resp := &http.Response{}
	resp.Request = r
	resp.TransferEncoding = r.TransferEncoding
	resp.Header = make(http.Header)
	resp.Header.Add("Content-Type", contentType)
	resp.StatusCode = status
	resp.Status = http.StatusText(status)
	buf := bytes.NewBufferString(body)
	resp.ContentLength = int64(buf.Len())
	resp.Body = ioutil.NopCloser(buf)
	return resp
}

func TextResponse(r *http.Request, text string) *http.Response {
	return NewResponse(r, ContentTypeText, http.StatusAccepted, text)
}
