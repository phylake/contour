// utility to sort various objects
package contour

import (
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
)

type endpointByIP []v1.EndpointAddress

func (e endpointByIP) Len() int           { return len(e) }
func (e endpointByIP) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e endpointByIP) Less(i, j int) bool { return e[i].IP < e[i].IP }

// TLS-enabled, so assumes a secret is attached
type filterChainTLSBySecretName []*envoy_api_v2_listener.FilterChain

func (fc filterChainTLSBySecretName) Len() int      { return len(fc) }
func (fc filterChainTLSBySecretName) Swap(i, j int) { fc[i], fc[j] = fc[j], fc[i] }
func (fc filterChainTLSBySecretName) Less(i, j int) bool {
	return envoy.RetrieveSecretName(fc[i].TlsContext) < envoy.RetrieveSecretName(fc[j].TlsContext)
}
