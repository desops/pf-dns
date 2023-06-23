package resolver

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"strconv"
	"syscall"

	"git.cadurx.com/pfdns/ipc"
)

// Main entry point for resolver subprocess
func Main(noChroot bool) {
	quitSig := make(chan os.Signal, 1)
	signal.Notify(quitSig, os.Interrupt, os.Kill, syscall.SIGTERM)
	reloadSig := make(chan os.Signal, 1)
	signal.Notify(reloadSig, syscall.SIGHUP)

	parentQuit := run(noChroot)
	for {
		select {
		case s := <-quitSig:
			log.Fatalf("resolver exiting: got sig %s", s)
		case s := <-reloadSig:
			log.Fatalf("resolver exiting: got sig %s", s)
		case <-parentQuit:
			log.Fatalf("parent quit")
		}
	}
}

func run(noChroot bool) chan bool {
	parentQuit := make(chan bool)

	parentPipe := os.NewFile(3, "read parent pipe")
	parentWrite := os.NewFile(4, "write parent pipe")
	resolv := os.NewFile(5, "resolvConfFile")
	config := os.NewFile(6, "configFile")

	// send IPC to our parent
	i := &ipc.IPC{}
	i.Writer(parentWrite)

	if !noChroot {
		u, err := user.Lookup("nobody")
		if err != nil {
			i.WriteFatal(err)
		}

		err = syscall.Chroot("/var/empty")
		if err != nil {
			i.WriteFatal(fmt.Errorf("could not chroot /var/empty: %s", err))
		}
		err = syscall.Chdir("/")
		if err != nil {
			i.WriteFatal(fmt.Errorf("could not chdir /: %s", err))
		}

		// well that's dumb, this don't work in linux
		if runtime.GOOS != "linux" {
			id, _ := strconv.Atoi(u.Gid)
			err = syscall.Setgid(id)
			if err != nil {
				i.WriteFatal(fmt.Errorf("setgid: %s", err))
			}
			id, _ = strconv.Atoi(u.Uid)
			err = syscall.Setuid(id)
			if err != nil {
				i.WriteFatal(fmt.Errorf("setuid: %s", err))
			}
		}
	}

	// needs __set_tcp :-(
	//pledge.Pledge("stdio inet", nil)

	// all chrooted, try parsing config
	dnscfg, cfg, err := loadConfig(resolv, config)
	if err != nil {
		i.WriteFatal(err)
	}
	_ = resolv.Close()
	_ = config.Close()

	go func() {
		for {
			_, _ = ioutil.ReadAll(parentPipe)
			parentQuit <- true
		}
	}()

	add := make(chan updateArgs, 100)
	go addPf(i, add)

	del := make(chan updateArgs, 100)
	go delPf(i, cfg, del)

	// startup complete, let our parent know so it will respawn us if we die
	ia := ipc.Args{
		Func: "startup",
	}
	i.Call(ia)

	for table, hosts := range cfg.Tables {
		//if *noFlush == false {
		flushTable(i, table)
		//}

		for _, host := range hosts {
			args := resolveArgs{
				add:     add,
				del:     del,
				quit:    parentQuit,
				table:   table,
				host:    host,
				verbose: cfg.Verbose,
				dnscfg:  dnscfg,
			}
			go resolve(args)
		}
	}

	return parentQuit
}

func loadConfig(dnsFile *os.File, cfgFile *os.File) (resolvConf, config, error) {
	dnscfg, err := resolvConfFromReader(dnsFile)
	if err != nil {
		return resolvConf{}, config{}, err
	}

	cfg, err := parseConfig(cfgFile)
	if err != nil {
		return resolvConf{}, config{}, err
	}
	//if *verbose {
	//	conf.Verbose = 2
	//}
	if cfg.Verbose > 0 {
		log.Printf("%+v", cfg)
	}

	return dnscfg, cfg, nil
}
