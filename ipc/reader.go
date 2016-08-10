package ipc

import (
	"bufio"
	"bytes"
	"log"
	"os"
)

// handles reading bits

func ipcError(args Args) {
	log.Printf("child had error: %s", args)
}

// Register a func with a callback
func (i *IPC) Register(f string, cb Func) {
	if i.subs == nil {
		i.subs = make(map[string]Func, 10)
		i.subs["error"] = ipcError
	}
	i.subs[f] = cb
}

// Reader usage go Reader(r), Reader now owns r (will call r.Close() on EOF)
// calls callbacks registered with Register()
func (i *IPC) Reader(r *os.File) {
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
				args.Func = name
				sub(args)
			} else {
				log.Printf("unknown ipc call: %s", name)
			}
		} else {
			log.Printf("got line %s", line)
		}
	}

	// we own r, so close it
	_ = r.Close()
	log.Printf("ipc reader done")
}
