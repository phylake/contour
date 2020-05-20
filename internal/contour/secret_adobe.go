package contour

import (
	"strings"

	"github.com/projectcontour/contour/internal/dag"
)

type dagSecretVisitor struct {
	secrets map[string]*dag.Secret
}

// visitSecretsAsDag() produces a map of *dag.Secret
// somewhat similar to visitSecrets()
func visitSecretsAsDag(root dag.Vertex) map[string]*dag.Secret {
	sv := dagSecretVisitor{
		secrets: make(map[string]*dag.Secret),
	}
	sv.visit(root)
	return sv.secrets
}

func (v *dagSecretVisitor) visit(vertex dag.Vertex) {
	switch svh := vertex.(type) {
	case *dag.SecureVirtualHost:
		if svh.Secret != nil {
			name := strings.Join([]string{svh.Secret.Namespace(), svh.Secret.Name()}, "/")
			if _, ok := v.secrets[name]; !ok {
				v.secrets[name] = svh.Secret
			}
		}
	default:
		vertex.Visit(v.visit)
	}
}
