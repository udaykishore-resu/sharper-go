// Package export is Technique 77: reuse a bytes.Buffer for
// scratch space; a strings.Builder is for one string.
package export

import (
	"bytes"
	"strconv"
)

type Transfer struct {
	ID, From, To string
	Amount       int64
}

// CSVWriter is not safe for concurrent use: keep one per
// goroutine.
type CSVWriter struct {
	buf bytes.Buffer
}

func (c *CSVWriter) Line(t Transfer) []byte {
	c.buf.Reset()
	c.buf.WriteString(t.ID)
	c.buf.WriteByte(',')
	c.buf.WriteString(t.From)
	c.buf.WriteByte(',')
	c.buf.WriteString(t.To)
	c.buf.WriteByte(',')
	b := c.buf.AvailableBuffer()
	b = strconv.AppendInt(b, t.Amount, 10)
	c.buf.Write(b)
	c.buf.WriteByte('\n')
	return c.buf.Bytes()
}
