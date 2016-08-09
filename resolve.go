package main

import (
	"log"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type resolveArgs struct {
	update chan updateArgs
	flush  chan bool
	quit   chan struct{}

	dnscfg *dns.ClientConfig

	table string
	host  string

	verbose uint8
}

func resolve(args resolveArgs) {
	// static IP address?
	if net.ParseIP(args.host) != nil {
		for {
			var addIP iPlist
			addIP.add(args.host)

			log.Printf("add %s:%s", args.table, strings.Join(addIP, ","))
			args.update <- updateArgs{addIP: addIP, remIP: nil, table: args.table}

			select {
			case <-args.flush:
				// reload now
			case <-args.quit:
				if args.verbose > 0 {
					log.Printf("stop %s", args.host)
				}
				return
			}
		}
	}

	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(args.host), dns.TypeA)
	m.RecursionDesired = true

	// if we fail, sleep longer and longer up to 10 min
	// XXX does not play nicely when one server is broken :-(
	var failTTL int64

	// we keep track of the last ips we added and remove them if they changed
	var remIP iPlist
	for {
		var addIP iPlist

		if args.verbose > 0 {
			log.Printf("resolve %s", args.host)
		}

		// recheck every 10 minutes regardless of dns TTL
		var minTTL int64 = 600

		for _, server := range args.dnscfg.Servers {
			r, _, err := c.Exchange(m, net.JoinHostPort(server, args.dnscfg.Port))

			if r == nil {
				log.Printf("exchange failed %s: %s", args.host, err)
				// slow down queries till we are retrying every 10 min
				failTTL += 30
				if failTTL > 600 {
					failTTL = 600
				}
				minTTL = failTTL
				continue
			}

			if r.Rcode != dns.RcodeSuccess {
				log.Printf("invalid answer %s", args.host)
				// slow down queries till we are retrying every 10 min
				failTTL += 30
				if failTTL > 600 {
					failTTL = 600
				}
				minTTL = failTTL
				continue
			}

			for _, ans := range r.Answer {
				if a, ok := ans.(*dns.A); ok {
					if args.verbose > 0 {
						//log.Printf("host %s -> %s, ttl %d\n", host, a.A, a.Hdr.Ttl)
					}

					if a.Hdr.Ttl < uint32(minTTL) {
						minTTL = int64(a.Hdr.Ttl)
					}

					var ip = a.A.String()
					addIP.add(ip)
					remIP.rem(ip)
				}
			}
		}

		// reset failTTL on success
		failTTL = 0

		log.Printf("add %s:%s ttl:%d %s, rem:%s", args.table, args.host, minTTL, strings.Join(addIP, ","), strings.Join(remIP, ","))
		args.update <- updateArgs{addIP: addIP, remIP: remIP, table: args.table}
		remIP = addIP

		// run 1 second after it expires
		minTTL++

		select {
		case <-args.flush:
			remIP = nil
		case <-time.After(time.Duration(minTTL) * time.Second):
		case <-args.quit:
			if args.verbose > 0 {
				log.Printf("stop %s", args.host)
			}
			return
		}
	}
}
