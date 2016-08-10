package ipc

import "io"

// Func IPC calls should confirm to this interface
type Func func(Args)

// IPC type
type IPC struct {
	w io.Writer

	subs map[string]Func
}

// Args populate and then call IPC.Call()
type Args struct {
	Func string
	Argv []string
}
