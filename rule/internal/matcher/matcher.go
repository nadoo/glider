package matcher

type Matcher interface {
	Match(t interface{}) bool
}
