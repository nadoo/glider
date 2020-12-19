package matcher

import (
	"github.com/nadoo/glider/rule/internal/trie"
	"strconv"
	"strings"
)

type CIDRMatcher trie.Trie

// TODO: ipv6
func NewCIDRMatcher(CIDRs []string) *CIDRMatcher {
	dict := make([]string, 0, len(CIDRs))
	for _, CIDR := range CIDRs {
		grp := strings.SplitN(CIDR, "/", 2)
		if len(grp) == 1 {
			grp = append(grp, "32")
		}
		l, _ := strconv.Atoi(grp[1])
		arr := strings.Split(grp[0], ".")
		var builder strings.Builder
		for _, sec := range arr {
			itg, _ := strconv.Atoi(sec)
			tmp := strconv.FormatInt(int64(itg), 2)
			builder.WriteString(strings.Repeat("0", 8-len(tmp)))
			builder.WriteString(tmp)
			if builder.Len() >= l {
				break
			}
		}
		dict = append(dict, builder.String()[:l])
	}
	return (*CIDRMatcher)(trie.New(dict))
}

func (m CIDRMatcher) Match(t interface{}) bool {
	return (*trie.Trie)(&m).Match(t.(string)) != ""
}
