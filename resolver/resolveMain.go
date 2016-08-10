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

	"git.cadurx.com/pf_dns_update/ipc"
	"git.cadurx.com/pf_dns_update/pledge"
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

	i := &ipc.IPC{}
	i.Writer(parentWrite)

	dnscfg, cfg, err := loadConfig(resolv, config)
	if err != nil {
		i.WriteFatal(err)
	}
	_ = resolv.Close()
	_ = config.Close()

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

	pledge.Pledge("stdio inet", nil)

	go func() {
		for {
			_, _ = ioutil.ReadAll(parentPipe)
			parentQuit <- true
		}
	}()

	uc := make(chan updateArgs, 100)
	go updatePf(i, uc)

	// startup complete
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
				update:  uc,
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

func updatePf(i *ipc.IPC, uc chan updateArgs) {
	for {
		u := <-uc

		if len(u.delIP) > 0 {
			var del []string
			del = append(del, u.table)
			del = append(del, u.delIP...)
			args := ipc.Args{
				Func: "delToTable",
				Argv: del,
			}
			i.Call(args)
		}

		if len(u.addIP) > 0 {
			var add []string
			add = append(add, u.table)
			add = append(add, u.addIP...)
			args := ipc.Args{
				Func: "addToTable",
				Argv: add,
			}
			i.Call(args)
		}
	}
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

func flushTable(i *ipc.IPC, table string) {
	args := ipc.Args{
		Func: "flushTable",
		Argv: []string{table},
	}
	i.Call(args)
}
