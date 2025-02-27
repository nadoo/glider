package obfs

import (
	"math/rand/v2"
)

func init() {
	register("http_post", newHttpPost)
}

// newHttpPost create a http_post object
func newHttpPost() IObfs {
	// newHttpSimple create a http_simple object

	t := &httpSimplePost{
		userAgentIndex: rand.IntN(len(requestUserAgent)),
		methodGet:      false,
	}
	return t
}
