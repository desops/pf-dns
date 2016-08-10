package ipc

import (
	"bufio"
	"bytes"
	"io"
	"log"
)

type IPCSub func(Args)

type IPC struct {
	w io.Writer

	subs map[string]IPCSub
}

type Args struct {
	Sub  string
	Argv []string
}

func ipcError(args Args) {
	log.Printf("child had error: %s", args)
}

func (i *IPC) Register(sub string, cb IPCSub) {
	if i.subs == nil {
		i.subs = make(map[string]IPCSub, 10)
		i.subs["error"] = ipcError
	}
	i.subs[sub] = cb
}

func (i *IPC) Reader(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		fields := bytes.Split(line, []byte{0})

		if len(fields) > 0 && len(string(fields[0])) > 1 {
			name := string(fields[0])

			sub, ok := i.subs[name]
			if ok {
				args := Args{}
				for i, a := range fields {
					if i == 0 {
						continue
					}
					args.Argv = append(args.Argv, string(a))
				}
				args.Sub = name
				sub(args)
			} else {
				log.Printf("unknown ipc call: %s", name)
			}
		} else {
			log.Printf("got line %s", line)
		}
	}

	log.Printf("ipc reader done")
}

func (i *IPC) Writer(w io.Writer) {
	i.w = w
}

func (i *IPC) WriteFatal(err error) {
	i.w.Write([]byte("error"))
	i.w.Write([]byte{0})
	i.w.Write([]byte(err.Error()))
	i.w.Write([]byte{'\n'})
	log.Fatal(err)
}

func (i *IPC) Call(arg Args) {
	var buf []byte
	buf = append(buf, []byte(arg.Sub)...)
	buf = append(buf, 0)
	for _, a := range arg.Argv {
		buf = append(buf, []byte(a)...)
		buf = append(buf, 0)
	}
	buf = append(buf, '\n')
	cnt, err := i.w.Write(buf)

	if err != nil {
		log.Fatal("write failed: %s", err)
	}

	if cnt != len(buf) {
		log.Fatal("wrote %d of %d, goodbye", cnt, len(buf))
	}
}
