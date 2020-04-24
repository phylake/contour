package dag

import (
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
)

// MaxProtoVersion - similar to MinProtoVersion, but for a max
// Note: upstream hard-code the version here, but internally uses TLS_AUTO
// Then they have to configure the version in all their tests. We do what they
// intended to do and return TLS_AUTO here; actual config happens in DownstreamTLSContext (internal/envoy)
func MaxProtoVersion(version string) envoy_api_v2_auth.TlsParameters_TlsProtocol {
	switch version {
	case "1.2":
		return envoy_api_v2_auth.TlsParameters_TLSv1_2
	default:
		return envoy_api_v2_auth.TlsParameters_TLS_AUTO
	}
}
