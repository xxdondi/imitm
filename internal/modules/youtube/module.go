// Package youtube blocks the observed iOS preroll paths by forcing plaintext
// player fallback and removing guarded protobuf ad fields from those replies.
package youtube

import (
	"github.com/elazarl/goproxy"
	"github.com/xxdondi/imitm/internal/modules"
)

type Module struct {
	evidence modules.Evidence
}

// New creates the combined YouTube module with the application's evidence sink.
func New(evidence modules.Evidence) *Module {
	return &Module{evidence: evidence}
}

func (*Module) Name() string { return "youtube" }

// Register must run after capture handlers to preserve original upstream bodies.
func (m *Module) Register(proxy *goproxy.ProxyHttpServer) {
	registerInitplayback502(proxy, m.evidence)
	registerPlayerAds(proxy, m.evidence)
}

var _ modules.Module = (*Module)(nil)
