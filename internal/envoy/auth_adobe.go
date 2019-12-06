package envoy

import (
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
)

// Retrieve the secret name attached to the given DownstreamTlsContext
// Since we created it, we know there's only one!
func RetrieveSecretName(tlsContext *envoy_api_v2_auth.DownstreamTlsContext) string {
	return tlsContext.CommonTlsContext.TlsCertificateSdsSecretConfigs[0].Name
}
