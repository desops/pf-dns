package resolver

import (
	"log"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type resolveArgs struct {
	add   chan updateArgs
	del   chan updateArgs
	flush chan bool
	quit  chan bool

	dnscfg resolvConf

	table string
	host  string

	verbose uint8
}

func resolve(args resolveArgs) {
	// static IP address?
	if net.ParseIP(args.host) != nil {
		doStatic(args)
		return
	}

	c := dns.Client{}
	m := dns.Msg{}
	m.SetQuestion(dns.Fqdn(args.host), dns.TypeA)
	m.RecursionDesired = true

	// if we fail, sleep longer and longer up to 10 min
	failTTL := make([]int64, len(args.dnscfg.servers))

	// we keep track of the last ips we added and remove them if they changed
	var curIP iPlist
	for {
		var gotIP iPlist

		if args.verbose > 0 {
			log.Printf("resolve %s", args.host)
		}

		// recheck every 10 minutes, even if the dns TTL says we could cache
		// for longer
		var minTTL int64 = 600

		for idx, server := range args.dnscfg.servers {
			respIP := resolv(server, c, &m, args, &failTTL[idx], &minTTL)

			for _, ip := range respIP {
				gotIP.add(ip)
			}
		}

		// only add/remove if we got IPs to add
		// for example if networking went down for a second, we don't want to remove old ips
		if len(gotIP) > 0 {
			curIP = _updatePf(args, minTTL, gotIP, curIP)

			// try again 1s after the TTL expires
			minTTL++
		}

		select {
		case <-time.After(time.Duration(minTTL) * time.Second):
			// re-resolv
		case <-args.flush:
			if args.verbose > 1 {
				log.Printf("flush %s", args.host)
			}
			curIP = nil
		case <-args.quit:
			if args.verbose > 0 {
				log.Printf("stop %s", args.host)
			}
			return
		}
	}
}

// returns a list of resolved ips
func resolv(server string, c dns.Client, m *dns.Msg, args resolveArgs, failTTL *int64, minTTL *int64) iPlist {
	var gotIP iPlist

	r, _, err := c.Exchange(m, net.JoinHostPort(server, "53"))
	if r == nil {
		log.Printf("exchange failed %s: %s", args.host, err)
		_bumpfail(failTTL, minTTL)
		return gotIP
	}

	if r.Rcode != dns.RcodeSuccess {
		log.Printf("invalid answer %s", args.host)
		_bumpfail(failTTL, minTTL)
		return gotIP
	}

	for _, ans := range r.Answer {
		if a, ok := ans.(*dns.A); ok {
			if args.verbose > 1 {
				log.Printf("host %s -> %s, ttl %d\n", args.host, a.A, a.Hdr.Ttl)
			}

			if a.Hdr.Ttl < uint32(*minTTL) {
				*minTTL = int64(a.Hdr.Ttl)
			}

			var ip = a.A.String()
			gotIP.add(ip)

			// reset failTTL for this server on any success
			*failTTL = 0
		}
	}

	return gotIP
}

func _bumpfail(failTTL *int64, minTTL *int64) {
	// slow down queries till we are retrying every 10 min
	*failTTL += 30
	if *failTTL > 600 {
		*failTTL = 600
	}
	*minTTL = *failTTL
}

func _updatePf(args resolveArgs, minTTL int64, gotIP iPlist, curIP iPlist) iPlist {
	var addIP iPlist
	var delIP iPlist

	// start off by assuming we need to delete all current ips
	delIP = append(delIP, curIP...)
	for _, ip := range gotIP {

		// don't have the IP? add it
		if delIP.contains(ip) == false {
			addIP = append(addIP, ip)
		} else {
			// don't delete it if our new response contains the "old" IP
			delIP.rem(ip)
		}
	}

	if len(addIP) > 0 {
		log.Printf("add %s:%s ttl:%d %s, del:%s, l:%s, g:%s", args.table, args.host, minTTL, addIP, delIP, curIP, gotIP)

		// send off IPC message to parent
		args.add <- updateArgs{ips: addIP, table: args.table}

		if len(delIP) > 0 {
			args.del <- updateArgs{ips: delIP, table: args.table}
		}

		// update our curIP to all the ones we "got" this round
		return gotIP
	}

	if args.verbose > 1 {
		log.Printf("no diff %s:%s ttl:%d %s, del:%s, l:%s, g:%s", args.table, args.host, minTTL, addIP, delIP, curIP, gotIP)
	}

	// no changes, keep our current list of IPs
	return curIP
}

func doStatic(args resolveArgs) {
	for {
		var addIP iPlist
		addIP.add(args.host)

		log.Printf("add %s:%s", args.table, strings.Join(addIP, ","))
		args.add <- updateArgs{ips: addIP, table: args.table}

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
