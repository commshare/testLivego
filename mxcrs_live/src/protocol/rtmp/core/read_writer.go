package core

import (
	"bufio"
	"io"
)

type ReadWriter struct {
	*bufio.ReadWriter
	readError  error
	writeError error
}
/*reader and writer 's buffer size is at least bufSize (4k)*/
func NewReadWriter(rw io.ReadWriter, bufSize int) *ReadWriter {
	return &ReadWriter{
		ReadWriter: bufio.NewReadWriter(bufio.NewReaderSize(rw, bufSize), bufio.NewWriterSize(rw, bufSize)),
	}
}

func (rw *ReadWriter) Read(p []byte) (int, error) {
	if rw.readError != nil {
		return 0, rw.readError
	}
	n, err := io.ReadAtLeast(rw.ReadWriter, p, len(p))
	rw.readError = err
	return n, err
}

func (rw *ReadWriter) ReadError() error {
	return rw.readError
}
/*TODO n bytes*/
func (rw *ReadWriter) ReadUintBE(n int) (uint32, error) {
	if rw.readError != nil {
		return 0, rw.readError
	}
	ret := uint32(0)  /*result returned in uint32*/
	for i := 0; i < n; i++ { /*n bytes in Big Endian*/
		b, err := rw.ReadByte() /*in go bufio.go ,which read one byte*/
		if err != nil {
			rw.readError = err
			return 0, err
		}
		ret = ret<<8 + uint32(b)  /**/
	}
	return ret, nil
}
/*n standands for n bytes, read n uint in little endian*/
func (rw *ReadWriter) ReadUintLE(n int) (uint32, error) {
	if rw.readError != nil {
		return 0, rw.readError
	}
	ret := uint32(0)
	for i := 0; i < n; i++ {
		b, err := rw.ReadByte()
		if err != nil {
			rw.readError = err
			return 0, err
		}
		ret += uint32(b) << uint32(i*8)  /*read from b []byte: n=0 ,return uint32(b[0]) ; n =1 , return b[1]<<8 + b[0] = b[1]*256+b[0]*/
	}
	return ret, nil
}

func (rw *ReadWriter) Flush() error {
	if rw.writeError != nil {
		return rw.writeError
	}

	if rw.ReadWriter.Writer.Buffered() == 0 {
		return nil
	}
	return rw.ReadWriter.Flush()
}

func (rw *ReadWriter) Write(p []byte) (int, error) {
	if rw.writeError != nil {
		return 0, rw.writeError
	}
	return rw.ReadWriter.Write(p)
}

func (rw *ReadWriter) WriteError() error {
	return rw.writeError
}

func (rw *ReadWriter) WriteUintBE(v uint32, n int) error {
	if rw.writeError != nil {
		return rw.writeError
	}
	for i := 0; i < n; i++ {
		b := byte(v>>uint32((n-i-1)<<3)) & 0xff /*construct a new byte （8 bits）*/
		if err := rw.WriteByte(b); err != nil {
			rw.writeError = err
			return err
		}
	}
	return nil
}

func (rw *ReadWriter) WriteUintLE(v uint32, n int) error {
	if rw.writeError != nil {
		return rw.writeError
	}
	for i := 0; i < n; i++ {
		b := byte(v) & 0xff
		if err := rw.WriteByte(b); err != nil {
			rw.writeError = err
			return err
		}
		v = v >> 8
	}
	return nil
}
