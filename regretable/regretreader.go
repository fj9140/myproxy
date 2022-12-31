package regretable

import "io"

type RegretableReader struct {
	reader   io.Reader
	overflow bool
	r, w     int
	buf      []byte
}

var defaultBufferSize = 500

type RegretableReaderCloser struct {
	RegretableReader
	c io.Closer
}

func (rbc *RegretableReaderCloser) Close() error {
	return rbc.c.Close()
}

func NewRegretableReaderCloser(rc io.ReadCloser) *RegretableReaderCloser {
	return &RegretableReaderCloser{*NewRegretableReader(rc), rc}
}

func NewRegretableReaderCloserSize(rc io.ReadCloser, size int) *RegretableReaderCloser {
	return &RegretableReaderCloser{*NewRegretableReaderSize(rc, size), rc}
}

func NewRegretableReaderSize(r io.Reader, size int) *RegretableReader {
	return &RegretableReader{reader: r, buf: make([]byte, size)}
}

func NewRegretableReader(r io.Reader) *RegretableReader {
	return NewRegretableReaderSize(r, defaultBufferSize)
}

func (rb *RegretableReader) Regret() {
	if rb.overflow {
		panic("regreting after overflow make no sense")
	}
	rb.r = 0
}

func (rb *RegretableReader) Forget() {
	if rb.overflow {
		panic("forgetting after overflow makes no sense")
	}

	rb.r = 0
	rb.w = 0
}

func (rb *RegretableReader) Read(p []byte) (n int, err error) {
	if rb.overflow {
		return rb.reader.Read(p)
	}
	if rb.r < rb.w {
		n = copy(p, rb.buf[rb.r:rb.w])
		rb.r += n
		return
	}
	n, err = rb.reader.Read(p)
	bn := copy(rb.buf[rb.w:], p[:n])
	rb.w, rb.r = rb.w+bn, rb.w+n
	if bn < n {
		rb.overflow = true
	}
	return
}
