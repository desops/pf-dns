package ipc

import (
	"io"
	"log"
)

// handles writing/calling

// Writer sets where to write to on IPC.Call()
func (i *IPC) Writer(w io.Writer) {
	i.w = w
}

// WriteFatal a fatal error, then die
func (i *IPC) WriteFatal(err error) {
	// ignore errors, not much we can do anway
	_, _ = i.w.Write([]byte("error"))
	_, _ = i.w.Write([]byte{0})
	_, _ = i.w.Write([]byte(err.Error()))
	_, _ = i.w.Write([]byte{'\n'})
	log.Fatal(err)
}

// Call execute IPC on Writer
func (i *IPC) Call(arg Args) {
	var buf []byte
	buf = append(buf, []byte(arg.Func)...)
	buf = append(buf, 0)
	for _, a := range arg.Argv {
		buf = append(buf, []byte(a)...)
		buf = append(buf, 0)
	}
	buf = append(buf, '\n')
	cnt, err := i.w.Write(buf)

	if err != nil {
		log.Fatalf("write failed: %s", err)
	}

	if cnt != len(buf) {
		log.Fatalf("wrote %d of %d, goodbye", cnt, len(buf))
	}
}
