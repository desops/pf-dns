package main

import (
	"log"
	"os/exec"
)

type updateArgs struct {
	addIP iPlist
	remIP iPlist
	table string
}

func flushPf(table string) {
	log.Printf("flushing table %s", table)

	cmd := exec.Command("/sbin/pfctl", "-q", "-t", table, "-T", "flush")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("pfctl %s: %s", err, out)
	}
}

func updatePf(uc chan updateArgs) {
	for {
		u := <-uc

		if len(u.remIP) > 0 {
			args := []string{"-t", u.table, "-T", "rem"}
			args = append(args, u.remIP...)

			cmd := exec.Command("/sbin/pfctl", args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("pfctl %s %s: %s", args, err, out)
			}
		}

		if len(u.addIP) > 0 {
			args := []string{"-t", u.table, "-T", "add"}
			args = append(args, u.addIP...)

			cmd := exec.Command("/sbin/pfctl", args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("pfctl %s %s: %s", args, err, out)
			}
		}
	}
}
