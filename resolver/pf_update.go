package resolver

import (
	"log"
	"sync"
	"time"

	"git.cadurx.com/pfdns/ipc"
)

type updateArgs struct {
	table string
	ips   iPlist
}

// we only delete ips after deleteExpire time
var deleteMU sync.Mutex
var deleteQueue = make(map[string]map[string]time.Time)

func delPf(i *ipc.IPC, cfg config, uc chan updateArgs) {
	var expDur time.Duration

	if len(cfg.DeleteAfter) > 0 {
		var err error
		expDur, err = time.ParseDuration(cfg.DeleteAfter)
		if err != nil {
			log.Printf("could not parse DeleteAfter, using default")
		}
	}

	if expDur == 0 {
		expDur = time.Minute * 1
	}

	var nextTime = time.Now().Add(60 * time.Minute)
	nextTimeout := time.NewTimer(nextTime.Sub(time.Now()))

	for {
		select {
		case u := <-uc:
			if len(u.ips) == 0 {
				return
			}

			exp := time.Now().Add(expDur)

			deleteMU.Lock()
			table, ok := deleteQueue[u.table]
			if !ok {
				table = make(map[string]time.Time)
				deleteQueue[u.table] = table
			}

			for _, ip := range u.ips {
				table[ip] = exp
			}
			deleteMU.Unlock()

			if exp.Sub(nextTime) <= 0 {
				nextTime = exp
				nextTimeout.Reset(exp.Sub(time.Now()))
			}

		case <-nextTimeout.C:
			now := time.Now()
			minexp := now.Add(60 * time.Minute)

			deleteMU.Lock()
			for table, ent := range deleteQueue {

				var del []string
				del = append(del, table)
				for ip, exp := range ent {

					// expired?
					if exp.Sub(now) <= 1*time.Second {
						del = append(del, ip)
						delete(ent, ip)
					} else {
						// set minexp to the next min expire time
						if minexp.After(exp) {
							minexp = exp
						}
					}
				}

				if len(del) > 1 {
					args := ipc.Args{
						Func: "delToTable",
						Argv: del,
					}
					i.Call(args)
				}
			}
			deleteMU.Unlock()

			nextTime = minexp
			nextTimeout.Reset(minexp.Sub(time.Now()))
		}
	}
}

func addPf(i *ipc.IPC, uc chan updateArgs) {
	for {
		u := <-uc

		if len(u.ips) == 0 {
			return
		}
		var add []string
		add = append(add, u.table)

		// remove addips from our delete queue
		deleteMU.Lock()
		del, ok := deleteQueue[u.table]
		if ok {
			for _, ip := range u.ips {
				_, ok := del[ip]
				if ok {
					delete(del, ip)
				}
			}
		}
		deleteMU.Unlock()

		add = append(add, u.ips...)
		args := ipc.Args{
			Func: "addToTable",
			Argv: add,
		}
		i.Call(args)
	}
}

func flushTable(i *ipc.IPC, table string) {
	args := ipc.Args{
		Func: "flushTable",
		Argv: []string{table},
	}
	i.Call(args)
}
