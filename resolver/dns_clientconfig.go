package resolver

import (
	"bufio"
	"io"
	"strings"
)

type resolvConf struct {
	servers []string
}

func resolvConfFromReader(r io.Reader) (resolvConf, error) {
	c := resolvConf{}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return c, err
		}
		line := scanner.Text()
		f := strings.Fields(line)
		if len(f) < 1 {
			continue
		}
		switch f[0] {
		case "nameserver":
			if len(f) > 1 {
				name := f[1]
				c.servers = append(c.servers, name)
			}

		}
	}

	return c, nil
}
