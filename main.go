package main

import (
	"flag"
	"log"
	"path/filepath"
	"syscall"

	"os"
	"os/signal"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/miekg/dns"
)

var cfgPath = flag.String("cfg", "./pf_update_dns.json", "config file path")
var noFlush = flag.Bool("noflush", false, "don't flush tables")
var resolvConf = flag.String("resolv", "/etc/resolv.conf", "resolv.conf path")
var verbose = flag.Bool("verbose", false, "verbose")

func main() {
	flag.Parse()

	dnscfg, cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("%s", err)
	}

	quit := launch(dnscfg, cfg)

	watcher := watchFiles()

	quitSig := make(chan os.Signal, 1)
	signal.Notify(quitSig, os.Interrupt, os.Kill, syscall.SIGTERM)

	reloadSig := make(chan os.Signal, 1)
	signal.Notify(reloadSig, syscall.SIGHUP)

	for {
		select {
		case s := <-quitSig:
			log.Fatalf("exiting: got sig %s", s)
		case s := <-reloadSig:
			log.Printf("got sig %s, reloading config", s)
			quit = reload(quit)
		case evt := <-watcher.Events:
			if evt.Name == *cfgPath || evt.Name == *resolvConf {
				if evt.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("%s modified, reloading", evt.Name)
					quit = reload(quit)
				}
			}
		case err := <-watcher.Errors:
			log.Printf("watcher err: %s", err)
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

func loadConfig() (*dns.ClientConfig, config, error) {
	dnscfg, err := dns.ClientConfigFromFile(*resolvConf)
	if err != nil {
		return nil, config{}, err
	}

	cfg := config{}
	err = cfg.Parse(*cfgPath)
	if err != nil {
		return nil, config{}, err
	}
	if *verbose {
		cfg.cfg.Verbose = 2
	}
	if cfg.cfg.Verbose > 0 {
		log.Printf("%+v", cfg.cfg)
	}

	return dnscfg, cfg, nil
}

func launch(dnscfg *dns.ClientConfig, cfg config) chan struct{} {
	quit := make(chan struct{})

	uc := make(chan updateArgs, 100)
	go updatePf(uc)

	var flush []chan bool
	for table, hosts := range cfg.cfg.Tables {

		if *noFlush == false {
			flushPf(table)
		}

		for _, host := range hosts {
			args := resolveArgs{
				update:  uc,
				flush:   make(chan bool),
				quit:    quit,
				table:   table,
				host:    host,
				verbose: cfg.cfg.Verbose,
				dnscfg:  dnscfg,
			}
			flush = append(flush, args.flush)
			go resolve(args)
		}
	}

	if cfg.cfg.Flush > 0 && *noFlush == false {
		go flusher(flush, quit, cfg)
	}

	return quit
}

func flusher(flush []chan bool, quit chan struct{}, cfg config) {
	for {
		select {
		case <-quit:
			return
		case <-time.After(time.Duration(cfg.cfg.Flush) * time.Second):
			if cfg.cfg.Verbose > 0 {
				log.Printf("Flush interval expired")
			}
		}

		// flush the tables
		for table := range cfg.cfg.Tables {
			flushPf(table)
		}

		// signal all the resolvers to re-resolve
		for _, f := range flush {
			f <- true
		}
	}
}
