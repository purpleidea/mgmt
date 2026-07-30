package strutil

import (
	"strings"
	"sync"
)

var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// Concat returns the strings that is build by concatenating its arguments. It is used as a cheaper/faster
// variant then [fmt.Sprintf] as it doesn't use reflection.
func Concat(sx ...string) string {
	sb := builderPool.Get().(*strings.Builder)
	for _, s := range sx {
		sb.WriteString(s)
	}
	s := sb.String()

	sb.Reset()
	builderPool.Put(sb)

	return s
}
