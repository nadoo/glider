package conflag

import (
	"bufio"
	"flag"
	"os"
	"path/filepath"
	"strings"
)

// Conflag .
type Conflag struct {
	*flag.FlagSet

	app     string
	osArgs  []string
	cfgFile string
	args    []string

	includes []string

	// TODO: add shorthand? or just use pflag?
	// shorthand map[byte]string
}

// New ...
func New(args ...string) *Conflag {
	if args == nil {
		args = os.Args
	}

	c := &Conflag{}
	c.app = args[0]
	c.osArgs = args[1:]
	c.FlagSet = flag.NewFlagSet(c.app, flag.ExitOnError)
	c.FlagSet.StringVar(&c.cfgFile, "config", "", "config file path")

	return c
}

// NewFromFile ...
func NewFromFile(app, cfgFile string) *Conflag {
	c := &Conflag{}

	if app != "" {
		c.app = app
	} else {
		c.app = os.Args[0]
	}

	c.cfgFile = cfgFile
	c.FlagSet = flag.NewFlagSet(c.app, flag.ExitOnError)

	c.StringSliceUniqVar(&c.includes, "include", nil, "include file")

	return c
}

// Parse ...
func (c *Conflag) Parse() (err error) {
	// parse 1st time and see whether there is a conf file.
	err = c.FlagSet.Parse(c.osArgs)
	if err != nil {
		return err
	}

	// if there is no args, just try to load the app.conf file.
	if c.cfgFile == "" && len(c.osArgs) == 0 {
		// trim app exetension
		for i := len(c.app) - 1; i >= 0 && c.app[i] != '/' && c.app[i] != '\\'; i-- {
			if c.app[i] == '.' {
				c.cfgFile = c.app[:i]
				break
			}
		}

		if c.cfgFile == "" {
			c.cfgFile = c.app
		}

		c.cfgFile += ".conf"
	}

	if c.cfgFile == "" {
		return nil
	}

	fargs, err := parseFile(c.cfgFile)
	if err != nil {
		return err
	}

	c.args = fargs
	c.args = append(c.args, c.osArgs...)

	// parse 2nd time to get the include file values
	err = c.FlagSet.Parse(c.args)
	if err != nil {
		return err
	}

	dir := filepath.Dir(c.cfgFile)

	// parse 3rd time to parse flags in include file
	for _, include := range c.includes {
		include = filepath.Join(dir, include)
		fargs, err := parseFile(include)
		if err != nil {
			return err
		}

		c.args = fargs
		c.args = append(c.args, c.osArgs...)

		err = c.FlagSet.Parse(c.args)
	}

	return err
}

func parseFile(cfgFile string) ([]string, error) {
	var s []string

	fp, err := os.Open(cfgFile)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if len(line) == 0 || line[:1] == "#" {
			continue
		}
		s = append(s, "-"+line)
	}

	return s, nil
}

// AppDir returns the app dir
func (c *Conflag) AppDir() string {
	return filepath.Dir(os.Args[0])
}

// ConfDir returns the config file dir
func (c *Conflag) ConfDir() string {
	return filepath.Dir(c.cfgFile)
}
