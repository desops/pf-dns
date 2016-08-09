package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
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
	defer func() { _ = fh.Close() }()

	blob, err := ioutil.ReadAll(fh)
	if err != nil {
		return err
	}

	// poor mans stripping of comments
	var re = regexp.MustCompile("//.*\n")
	blob = re.ReplaceAll(blob, []byte(""))

	j := &configJSON{}
	err = json.Unmarshal(blob, &j)
	if err != nil {
		return fmt.Errorf("bad json in file %s: %s", path, err)
	}
	cfg.cfg = *j
	return nil
}
