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
var noChroot = flag.Bool("nochroot", false, "disable chroot/setuid(nobody)")

// are we a resolver process?
var isResolver = flag.Int("resolver", 0, "internal flag")

type resolverState struct {
	quit chan error
	proc *os.Process
}

func main() {
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// resolver subprocess?
	if *isResolver > 0 {
		resolver.Main(*noChroot)
		return
	}

	// register IPC callbacks for resolver subprocess to call
	i := &ipc.IPC{}
	pfIPCInit(i)

	quitSig := make(chan os.Signal, 1)
	signal.Notify(quitSig, os.Interrupt, os.Kill, syscall.SIGTERM)
	reloadSig := make(chan os.Signal, 1)
	signal.Notify(reloadSig, syscall.SIGHUP)

	// start the resolver subprocess
	resolverState := startResolver(i)

	// reload if resolvConf or cfgFile files change
	watcher := watchFiles()

	// we need __set_tcb and we have no way to get it :-(
	// pledge.Pledge("stdio proc exec rpath", nil)

	for {
		select {
		case s := <-quitSig:
			log.Fatalf("exiting: got sig %s", s)
		case s := <-reloadSig:
			log.Printf("got sig %s, reloading config", s)

			// will respawn when we get <-resolverState.quit
			resolverState.proc.Kill()
		case evt := <-watcher.Events:
			if evt.Name == *cfgPath || evt.Name == *resolvConf {
				if evt.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("%s modified, reloading", evt.Name)

					// will respawn when we get <-resolverState.quit
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
		log.Printf("can't watch %s: %s", *resolvConf, err)
	}
	err = watcher.Add(filepath.Dir(*cfgPath))
	if err != nil {
		log.Printf("can't watch %s: %s", *cfgPath, err)
	}

	return watcher
}

func startResolver(i *ipc.IPC) resolverState {
	args := os.Args
	args = append(args, "-resolver", fmt.Sprintf("%d", os.Getpid()))

	// pipes for parent/child death detection
	// if the parent dies, the child will detect it via rp and exit
	rp, wp, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}

	// ipc pipes
	rcomp, wcomp, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}

	// open config and pass it to resolverProcess to parse after dropping privs/chrooting
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
	_ = resolv.Close()

	// start processing IPC for our subprocess
	go i.Reader(rcomp)

	// detect child death
	var childQuit = make(chan error)
	go func() {
		ps, err := proc.Wait()
		wp.Close()

		if err == nil {
			childQuit <- fmt.Errorf("%s", ps.String())
		} else {
			childQuit <- err
		}
	}()

	return resolverState{
		quit: childQuit,
		proc: proc,
	}
}
