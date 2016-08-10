package resolver

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
)

// {"Tables": {"pf_table": ["hostname1", "hostname2"...]}}
type configJSON struct {
	Tables  map[string][]string
	Flush   uint32
	Verbose uint8
	Chroot  string
	User    string
}

type config struct {
	cfg configJSON
}

func (cfg *config) Parse(r io.Reader) error {
	blob, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	// poor mans stripping of comments
	var re = regexp.MustCompile("//.*\n")
	blob = re.ReplaceAll(blob, []byte(""))

	j := &configJSON{}
	err = json.Unmarshal(blob, &j)
	if err != nil {
		return fmt.Errorf("bad json in config: %s", err)
	}
	cfg.cfg = *j
	return nil
}
