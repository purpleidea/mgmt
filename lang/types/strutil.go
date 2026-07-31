package types

import (
	"strings"
	"sync"
)

type pool struct {
	sync.Pool
}

func (p *pool) Get() *strings.Builder   { return p.Pool.Get().(*strings.Builder) }
func (p *pool) Put(sb *strings.Builder) { sb.Reset(); p.Pool.Put(sb) }

var builderPool = &pool{
	Pool: sync.Pool{
		New: func() any {
			return &strings.Builder{}
		},
	},
}

// concat returns the strings that is build by concatenating its arguments. It is used as a cheaper/faster
// variant then [fmt.Sprintf] as it doesn't use reflection.
func concat(sx ...string) string {
	sb := builderPool.Get()
	defer builderPool.Put(sb)
	for _, s := range sx {
		sb.WriteString(s)
	}
	return sb.String()
}
