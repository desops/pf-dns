package resolver

import (
	"log"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type updateArgs struct {
	table string
	addIP iPlist
	delIP iPlist
}

type resolveArgs struct {
	update chan updateArgs
	flush  chan bool
	quit   chan bool

	dnscfg resolvConf

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
			args.update <- updateArgs{addIP: addIP, delIP: nil, table: args.table}

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
	failTTL := make([]int64, len(args.dnscfg.servers))

	// we keep track of the last ips we added and remove them if they changed
	var lastIP iPlist
	for {
		var gotIP iPlist

		if args.verbose > 0 {
			log.Printf("resolve %s", args.host)
		}

		// recheck every 10 minutes regardless of dns TTL
		var minTTL int64 = 600

		for idx, server := range args.dnscfg.servers {
			r, _, err := c.Exchange(m, net.JoinHostPort(server, "53"))

			if r == nil {
				log.Printf("exchange failed %s: %s", args.host, err)
				// slow down queries till we are retrying every 10 min
				failTTL[idx] += 30
				if failTTL[idx] > 600 {
					failTTL[idx] = 600
				}
				minTTL = failTTL[idx]
				continue
			}

			if r.Rcode != dns.RcodeSuccess {
				log.Printf("invalid answer %s", args.host)
				// slow down queries till we are retrying every 10 min
				failTTL[idx] += 30
				if failTTL[idx] > 600 {
					failTTL[idx] = 600
				}
				minTTL = failTTL[idx]
				continue
			}

			for _, ans := range r.Answer {
				if a, ok := ans.(*dns.A); ok {
					if args.verbose > 1 {
						log.Printf("host %s -> %s, ttl %d\n", args.host, a.A, a.Hdr.Ttl)
					}

					if a.Hdr.Ttl < uint32(minTTL) {
						minTTL = int64(a.Hdr.Ttl)
					}

					var ip = a.A.String()
					gotIP.add(ip)

					// reset failTTL for this server on any success
					failTTL[idx] = 0
				}
			}
		}

		// only add/remove if we got IPs to add
		// for example if networking went down for a second, we don't want to remove old ips
		if len(gotIP) > 0 {
			var addIP iPlist

			var rem iPlist
			copy(rem, lastIP)
			for _, ip := range gotIP {
				if rem.contains(ip) == false {
					addIP = append(addIP, ip)
				} else {
					rem.rem(ip)
				}
			}

			if len(addIP) > 0 {
				log.Printf("add %s:%s ttl:%d %s, rem:%s, l:%s, g:%s", args.table, args.host, minTTL, addIP, rem, lastIP, gotIP)
				args.update <- updateArgs{addIP: addIP, delIP: rem, table: args.table}
				lastIP = rem
			} else {
				if args.verbose > 1 {
					log.Printf("no diff %s:%s ttl:%d %s, rem:%s, l:%s, g:%s", args.table, args.host, minTTL, addIP, rem, lastIP, gotIP)
				}
			}

			// run 1 second after it expires
			minTTL++
		}

		select {
		case <-args.flush:
			if args.verbose > 1 {
				log.Printf("flush %s", args.host)
			}
			lastIP = nil
		case <-time.After(time.Duration(minTTL) * time.Second):
		case <-args.quit:
			if args.verbose > 0 {
				log.Printf("stop %s", args.host)
			}
			return
		}
	}
}
