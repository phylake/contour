package envoy

import (
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

// DownstreamTLSContextAdobe - same as upstream but handles tlsMaxProtoVersion
func DownstreamTLSContextAdobe(serverSecret *dag.Secret, tlsMinProtoVersion, tlsMaxProtoVersion envoy_api_v2_auth.TlsParameters_TlsProtocol, peerValidationContext *dag.PeerValidationContext, alpnProtos ...string) *envoy_api_v2_auth.DownstreamTlsContext {
	tls := DownstreamTLSContext(serverSecret, tlsMinProtoVersion, peerValidationContext, alpnProtos...)
	tls.CommonTlsContext.TlsParams.TlsMaximumProtocolVersion = tlsMaxProtoVersion
	return tls
}

// GetDownstreamTLSContext retrieves the DownstreamTlsContext from a FilterChain
func GetDownstreamTLSContext(fc *envoy_api_v2_listener.FilterChain) *envoy_api_v2_auth.DownstreamTlsContext {
	cfg := fc.GetTransportSocket().GetTypedConfig()
	if cfg != nil {
		return protobuf.MustUnmarshalAny(cfg).(*envoy_api_v2_auth.DownstreamTlsContext)
	}
	return nil
}
