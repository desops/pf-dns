package resolver

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
)

// {"Tables": {"pf_table": ["hostname1", "hostname2"...]}}
type config struct {
	Tables      map[string][]string
	Flush       uint32
	Verbose     uint8
	DeleteAfter string
}

func parseConfig(r io.Reader) (config, error) {
	blob, err := ioutil.ReadAll(r)
	if err != nil {
		return config{}, err
	}

	// poor mans stripping of comments
	var re = regexp.MustCompile("//.*\n")
	blob = re.ReplaceAll(blob, []byte(""))

	j := config{}
	err = json.Unmarshal(blob, &j)
	if err != nil {
		return j, fmt.Errorf("bad json in config: %s", err)
	}

	return j, nil
}
