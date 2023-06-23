package main

import (
	"log"
	"os/exec"
	"sync"

	"git.cadurx.com/pf-dns/ipc"
)

func pfIPCInit(i *ipc.IPC) {
	i.Register("flushTable", flushTable)
	i.Register("addToTable", addToTable)
	i.Register("delToTable", delToTable)
	i.Register("startup", startup)
}

// the resolver sends a startup message if it could start and process the config
// if there were syntax errors or somesuch, we want to exit. which we detect by
// seeing if the resolverStarted() in our main process, if so we respawn it,
// otherwise we will assume we couldn't start the resolver and treat it as a
// fatal error in the main process
var _resolverStarted bool
var _rlock sync.Mutex

func resolverStarted() bool {
	_rlock.Lock()
	defer _rlock.Unlock()
	return _resolverStarted
}

func startup(args ipc.Args) {
	_rlock.Lock()
	defer _rlock.Unlock()
	_resolverStarted = true
}

func flushTable(args ipc.Args) {
	log.Printf("flushing table %s", args.Argv[0])

	if *dry {
		return
	}

	cmd := exec.Command("/sbin/pfctl", "-q", "-t", args.Argv[0], "-T", "flush")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("pfctl %s: %s", err, out)
	}
}

func delToTable(args ipc.Args) {
	if len(args.Argv) <= 1 {
		return
	}

	if *dry {
		return
	}

	cargs := []string{"-t", args.Argv[0], "-T", "delete"}
	cargs = append(cargs, args.Argv[1:]...)

	cmd := exec.Command("/sbin/pfctl", cargs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("pfctl %s %s: %s", cargs, err, out)
	}
}

func addToTable(args ipc.Args) {
	if len(args.Argv) <= 1 {
		return
	}

	if *dry {
		return
	}

	cargs := []string{"-t", args.Argv[0], "-T", "add"}
	cargs = append(cargs, args.Argv[1:]...)

	cmd := exec.Command("/sbin/pfctl", cargs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("pfctl %s %s: %s", cargs, err, out)
	}
}
