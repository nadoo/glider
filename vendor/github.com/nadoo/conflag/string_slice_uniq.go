package conflag

type stringSliceUniqValue struct {
	*stringSliceValue
}

func newStringSliceUniqValue(val []string, p *[]string) *stringSliceUniqValue {
	return &stringSliceUniqValue{stringSliceValue: newStringSliceValue(val, p)}
}

func (s *stringSliceUniqValue) Set(val string) error {
	if !s.changed {
		*s.value = []string{val}
		s.changed = true
	}

	dup := false
	for _, v := range *s.value {
		if v == val {
			dup = true
		}
	}

	if !dup {
		*s.value = append(*s.value, val)
	}

	return nil
}

func (s *stringSliceUniqValue) Type() string {
	return "stringSliceUniq"
}

func (s *stringSliceUniqValue) String() string {
	return ""
}

// StringSliceUniqVar defines a string flag with specified name, default value, and usage string.
// The argument p points to a []string variable in which to store the value of the flag.
func (c *Conflag) StringSliceUniqVar(p *[]string, name string, value []string, usage string) {
	c.Var(newStringSliceUniqValue(value, p), name, usage)
}

// StringUniqSlice defines a string flag with specified name, default value, and usage string.
// The return value is the address of a []string variable that stores the value of the flag.
func (c *Conflag) StringUniqSlice(name string, value []string, usage string) *[]string {
	p := []string{}
	c.StringSliceUniqVar(&p, name, value, usage)
	return &p
}
