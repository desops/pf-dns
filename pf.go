package main

import (
	"log"
	"os/exec"

	"git.cadurx.com/pf_dns_update/ipc"
)

var resolverStarted bool

func pfIPCInit(i *ipc.IPC) {
	i.Register("flushTable", flushTable)
	i.Register("addToTable", addToTable)
	i.Register("delToTable", delToTable)
	i.Register("startup", startup)
}

func startup(args ipc.Args) {
	resolverStarted = true
}

func flushTable(args ipc.Args) {
	log.Printf("flushing table %s", args.Argv[0])

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
	cargs := []string{"-t", args.Argv[0], "-T", "add"}
	cargs = append(cargs, args.Argv[1:]...)

	cmd := exec.Command("/sbin/pfctl", cargs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("pfctl %s %s: %s", cargs, err, out)
	}
}
