package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// {"Tables": {"pf_table": ["hostname1", "hostname2"...]}}
type configJSON struct {
	Tables  map[string][]string
	Flush   uint32
	Verbose uint8
}

type config struct {
	cfg configJSON
}

func (cfg *config) Parse(path string) error {
	debugf("parsing config: %s", path)

	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	dec := json.NewDecoder(fh)

	j := &configJSON{}
	err = dec.Decode(&j)
	if err != nil {
		return fmt.Errorf("bad json in file %s: %s", path, err)
	}
	cfg.cfg = *j
	return nil
}
