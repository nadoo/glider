package proxy

import "strings"

var (
	msg    strings.Builder
	usages = make(map[string]string)
)

// AddUsage adds help message for the named proxy.
func AddUsage(name, usage string) {
	usages[name] = usage
	msg.WriteString(usage)
	msg.WriteString("\n--")
}

// Usage returns help message of the named proxy.
func Usage(name string) string {
	if name == "all" {
		return msg.String()
	}
	if usage, ok := usages[name]; ok {
		return usage
	}
	return "can not find usage for: " + name
}
