package rule

import (
	"fmt"
	"github.com/nadoo/glider/rule/internal/matcher"
	"github.com/v2rayA/routingA"
)

type OutType int
type DomainStrategy int
type Network int

const (
	OutDirect  = "direct"
	OutProxy   = "proxy"
	OutReject  = "reject"
	OutDefault = "_default" //try not to conflict with user's definition
)
const (
	DomainStrategyAsIs DomainStrategy = iota
	DomainStrategyIPIfNonMatch
	DomainStrategyIPOnDemand
)

type RoutingA struct {
	DefaultOut     string
	DomainStrategy DomainStrategy // TODO: not valid. there would be some changes for current code structure
	Rules          []RoutingRule
}

func NewRoutingA(routing routingA.RoutingA) (ra *RoutingA, err error) {
	ra = &RoutingA{
		DefaultOut:     OutProxy,
		DomainStrategy: DomainStrategyIPIfNonMatch,
	}
	rs, ds := routing.Unfold()
	for _, d := range ds {
		switch d.Name {
		case "default":
			ra.DefaultOut = d.Value.(string)
		case "domainStrategy":
			switch d.Value.(string) {
			case "AsIs":
				ra.DomainStrategy = DomainStrategyAsIs
			case "IPIfNonMatch":
				ra.DomainStrategy = DomainStrategyIPIfNonMatch
			case "IPOnDemand":
				ra.DomainStrategy = DomainStrategyIPOnDemand
			default:
				return nil, fmt.Errorf("unsupported domainStrategy: %v", d.Value)
			}
		}
	}
	for _, r := range rs {
		var rr RoutingRule
		rr.Out = r.Out
		for _, cond := range r.And {
			m := Matcher{
				RuleType: cond.Name,
			}
			switch cond.Name {
			case "domain":
				m.Matcher = matcher.NewSuffixDomainTree(cond.Params)
			case "tip", "sip":
				m.Matcher = matcher.NewCIDRMatcher(cond.Params)
			case "network":
				m.Matcher = matcher.NewNetworkMatcher(cond.Params)
			case "app", "tport", "sport":
				m.Matcher = matcher.NewStringMatcher(cond.Params)
			default:
				continue
			}
			rr.Matchers = append(rr.Matchers, m)
		}
		ra.Rules = append(ra.Rules, rr)
	}
	return ra, nil
}

//TODO
//func (ra *RoutingA) MergeAdjacentRules() {
//}

type Matcher struct {
	matcher.Matcher
	RuleType string
}

type RoutingRule struct {
	Out      string
	Matchers []Matcher
}
