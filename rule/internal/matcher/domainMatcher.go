package matcher

import "strings"

type SuffixDomainTree map[string]interface{}

func NewSuffixDomainTree(domains []string) *SuffixDomainTree {
	var (
		tree = make(map[string]interface{})
		m    = tree
		ok   bool
	)
	for _, d := range domains {
		m = tree
		fields := strings.Split(d, ".")
		for i := len(fields) - 1; i >= 0; i-- {
			var t interface{}
			if t, ok = m[fields[i]]; ok {
				m = t.(map[string]interface{})
			} else {
				m[fields[i]] = make(map[string]interface{})
				m = m[fields[i]].(map[string]interface{})
			}
		}
		m[".end"] = struct{}{}
	}
	return (*SuffixDomainTree)(&tree)
}

func (m SuffixDomainTree) Match(t interface{}) bool {
	fields := strings.Split(t.(string), ".")
	mm := m
	for i := len(fields) - 1; i >= 0; i-- {
		var tt interface{}
		var ok bool
		if tt, ok = mm[fields[i]]; ok {
			mm = tt.(map[string]interface{})
			if _, ok := mm[".end"]; ok {
				return true
			}
		} else {
			return false
		}
	}
	return false
}
