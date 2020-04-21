package envoy

import (
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
)

// DownstreamTLSContextAdobe - same as upstream but handles tlsMaxProtoVersion
func DownstreamTLSContextAdobe(secretName string, tlsMinProtoVersion envoy_api_v2_auth.TlsParameters_TlsProtocol, tlsMaxProtoVersion envoy_api_v2_auth.TlsParameters_TlsProtocol, alpnProtos ...string) *envoy_api_v2_auth.DownstreamTlsContext {
	tls := DownstreamTLSContext(secretName, tlsMinProtoVersion, alpnProtos...)
	tls.CommonTlsContext.TlsParams.TlsMaximumProtocolVersion = tlsMaxProtoVersion
	return tls
}

// Retrieve the secret name and TLS protocol version attached to the given DownstreamTlsContext
// Since we created it, we know there's only secret!
func RetrieveSecretNameAndTLSVersions(transSocket *envoy_api_v2_core.TransportSocket) (string, envoy_api_v2_auth.TlsParameters_TlsProtocol, envoy_api_v2_auth.TlsParameters_TlsProtocol) {
	ctc := getDownstreamTlsContext(transSocket.GetTypedConfig()).CommonTlsContext
	return ctc.TlsCertificateSdsSecretConfigs[0].Name, ctc.TlsParams.TlsMinimumProtocolVersion, ctc.TlsParams.TlsMaximumProtocolVersion
}

func getDownstreamTlsContext(a *any.Any) *envoy_api_v2_auth.DownstreamTlsContext {
	tls := toMessage(a).(*envoy_api_v2_auth.DownstreamTlsContext)
	return tls
}

// toMessage is the reverse of envoy.toAny()
func toMessage(a *any.Any) proto.Message {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	if err != nil {
		panic(err.Error())
	}
	return x.Message
}
