package envoy

import (
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
)

// Retrieve the secret name attached to the given DownstreamTlsContext
// Since we created it, we know there's only one!
func RetrieveSecretName(transSocket *envoy_api_v2_core.TransportSocket) string {
	return getDownstreamTlsContext(transSocket.GetTypedConfig()).CommonTlsContext.TlsCertificateSdsSecretConfigs[0].Name
}

// Retrieve the min TLS protocol version configured on the given DownstreamTlsContext
func RetrieveMinTLSVersion(transSocket *envoy_api_v2_core.TransportSocket) envoy_api_v2_auth.TlsParameters_TlsProtocol {
	return getDownstreamTlsContext(transSocket.GetTypedConfig()).CommonTlsContext.TlsParams.TlsMinimumProtocolVersion
}

// Shortcut to get both previous values in 1 swoop
func RetrieveSecretNameAndMinTLSVersion(transSocket *envoy_api_v2_core.TransportSocket) (string, envoy_api_v2_auth.TlsParameters_TlsProtocol) {
	ctc := getDownstreamTlsContext(transSocket.GetTypedConfig()).CommonTlsContext
	return ctc.TlsCertificateSdsSecretConfigs[0].Name, ctc.TlsParams.TlsMinimumProtocolVersion
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
