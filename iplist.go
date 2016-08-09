package main

import "strings"

type iPlist []string

func (i *iPlist) contains(ip string) bool {
	for _, cip := range *i {
		if cip == ip {
			return true
		}
	}
	return false
}

func (i *iPlist) add(ip string) {
	if !i.contains(ip) {
		*i = append(*i, ip)
	}
}

func (i *iPlist) rem(ip string) {
	var nrem []string
	for _, nip := range *i {
		if nip != ip {
			nrem = append(nrem, nip)
		}
	}
	*i = nrem
}

func (i iPlist) String() string {
	return strings.Join(i, " ")
}
