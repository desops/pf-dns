package main

import "log"

func debugf(fmt string, v ...interface{}) {
	log.Printf(fmt, v)
}
