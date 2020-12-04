package matcher

type StringMatcher map[string]struct{}

func NewStringMatcher(app []string) *StringMatcher {
	m := make(map[string]struct{})
	for _, name := range app {
		m[name] = struct{}{}
	}
	if len(m) == 0 {
		return nil
	}
	return (*StringMatcher)(&m)
}

func (m StringMatcher) Match(t interface{}) bool {
	_, ok := m[t.(string)]
	return ok
}
