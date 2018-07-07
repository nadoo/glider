// source: https://github.com/spf13/pflag/blob/master/string_slice.go

package conflag

type stringSliceValue struct {
	value   *[]string
	changed bool
}

func newStringSliceValue(val []string, p *[]string) *stringSliceValue {
	ssv := new(stringSliceValue)
	ssv.value = p
	*ssv.value = val
	return ssv
}

func (s *stringSliceValue) Set(val string) error {
	if !s.changed {
		*s.value = []string{val}
		s.changed = true
	} else {
		*s.value = append(*s.value, val)
	}
	return nil
}

func (s *stringSliceValue) Type() string {
	return "stringSlice"
}

func (s *stringSliceValue) String() string {
	return ""
}

// StringSliceVar defines a string flag with specified name, default value, and usage string.
// The argument p points to a []string variable in which to store the value of the flag.
func (c *Conflag) StringSliceVar(p *[]string, name string, value []string, usage string) {
	c.Var(newStringSliceValue(value, p), name, usage)
}

// StringSlice defines a string flag with specified name, default value, and usage string.
// The return value is the address of a []string variable that stores the value of the flag.
func (c *Conflag) StringSlice(name string, value []string, usage string) *[]string {
	p := []string{}
	c.StringSliceVar(&p, name, value, usage)
	return &p
}
