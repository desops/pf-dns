package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"syscall"

	"os"
	"os/signal"

	"git.cadurx.com/pf_dns_update/ipc"
	"git.cadurx.com/pf_dns_update/resolver"

	"github.com/fsnotify/fsnotify"
	"github.com/kardianos/osext"
)

var cfgPath = flag.String("cfg", "./pf_dns_update.json", "config file path")
var noFlush = flag.Bool("noflush", false, "don't flush tables")
var resolvConf = flag.String("resolv", "/etc/resolv.conf", "resolv.conf path")
var verbose = flag.Bool("verbose", false, "verbose")

// are we a resolver process?
var isResolver = flag.Int("resolver", 0, "internal flag")

type resolverState struct {
	quit chan error
	proc *os.Process
}

func main() {
	flag.Parse()

	// resolver suubprocess?
	if *isResolver > 0 {
		resolver.Main()
		return
	}

	i := &ipc.IPC{}
	pfIPCInit(i)

	quitSig := make(chan os.Signal, 1)
	signal.Notify(quitSig, os.Interrupt, os.Kill, syscall.SIGTERM)
	reloadSig := make(chan os.Signal, 1)
	signal.Notify(reloadSig, syscall.SIGHUP)

	resolverState := startResolver(i)
	watcher := watchFiles()

	for {
		select {
		case s := <-quitSig:
			log.Fatalf("exiting: got sig %s", s)
		case s := <-reloadSig:
			log.Printf("got sig %s, reloading config", s)
			resolverState.proc.Kill()
		case evt := <-watcher.Events:
			if evt.Name == *cfgPath || evt.Name == *resolvConf {
				if evt.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("%s modified, reloading", evt.Name)
					resolverState.proc.Kill()
				}
			}
		case err := <-watcher.Errors:
			log.Printf("watcher err: %s", err)
		case err := <-resolverState.quit:
			// set by a startup IPC message in pf.go
			if resolverStarted() {
				log.Printf("resolver died: %s", err)
				resolverState = startResolver(i)
			} else {
				log.Fatalf("resolver died in init %s", err)
			}
		}
	}
}

func watchFiles() *fsnotify.Watcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	// we have to watch on the dir, editors rename/remove the old file
	err = watcher.Add(filepath.Dir(*resolvConf))
	if err != nil {
		log.Fatal(err)
	}
	err = watcher.Add(filepath.Dir(*cfgPath))
	if err != nil {
		log.Fatal(err)
	}

	return watcher
}

/*
func reload(oldQuit chan struct{}) chan struct{} {
	dnscfg, cfg, err := loadConfig()
	if err != nil {
		log.Printf("error loading config: %s", err)
		// got an error? do nothing
		return oldQuit
	}

	close(oldQuit)

	return launch(dnscfg, cfg)
}
*/

func startResolver(i *ipc.IPC) resolverState {
	args := os.Args
	args = append(args, "-resolver", fmt.Sprintf("%d", os.Getpid()))

	rp, wp, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	rcomp, wcomp, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}

	resolv, err := os.Open(*resolvConf)
	if err != nil {
		log.Fatal(err)
	}
	conf, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	attr := &os.ProcAttr{
		Files: []*os.File{
			os.Stdin,
			os.Stdout,
			os.Stderr,
			// should be stdin but gets screwed on linux or something
			rp,
			wcomp,
			resolv,
			conf,
		},
	}

	exe, err := osext.Executable()
	if err != nil {
		log.Fatal(err)
	}

	proc, err := os.StartProcess(exe, args, attr)
	if err != nil {
		log.Fatal(err)
	}

	// closed on exit so child can detect parent death
	_ = wp

	// not needed any longer
	_ = rp.Close()
	_ = wcomp.Close()
	_ = conf.Close()
	err = resolv.Close()
	if err != nil {
		log.Fatal(err)
	}

	go i.Reader(rcomp)

	var childQuit = make(chan error)
	go func() {
		_, err := proc.Wait()
		wp.Close()
		childQuit <- err
	}()

	return resolverState{
		quit: childQuit,
		proc: proc,
	}
}
