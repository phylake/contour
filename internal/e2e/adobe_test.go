// All tests func should start with TestAdobe (fixup is applied only to
// non-adobe tests; the name is used to make that determination)
//
// tests are organized by their name: TestAdobe{Cluster|Listener|Route}...
// this is so we can filter via "--run TestAdobeCluster" etc.

package e2e

import (
	"os"
	"testing"
	"time"

	udpa_type_v1 "github.com/cncf/udpa/go/udpa/type/v1"
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/projectcontour/contour/adobe"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ==== IngressRoute CRD customization ====
// == Route
// HashPolicy []HashPolicy `json:"hashPolicy,omitempty"`
// PerFilterConfig *PerFilterConfig `json:"perFilterConfig,omitempty"`
// Timeout *Duration `json:"timeout,omitempty"`
// IdleTimeout *Duration `json:"idleTimeout,omitempty"`
//
// == Service
// IdleTimeout *Duration `json:"idleTimeout,omitempty"`

func TestAdobeRouteHashPolicy(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hashpolicy.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				HashPolicy: []ingressroutev1.HashPolicy{
					{
						Header: &ingressroutev1.HashPolicyHeader{
							HeaderName: "x-some-header",
						},
					},
					{
						Cookie: &ingressroutev1.HashPolicyCookie{
							Name: "nom-nom-nom",
						},
						Terminal: true,
					},
					{
						ConnectionProperties: &ingressroutev1.HashPolicyConnectionProperties{
							SourceIp: true,
						},
					},
				},
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	r.Route.HashPolicy = make([]*envoy_api_v2_route.RouteAction_HashPolicy, 3)
	r.Route.HashPolicy[0] = &envoy_api_v2_route.RouteAction_HashPolicy{
		PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Header_{
			Header: &envoy_api_v2_route.RouteAction_HashPolicy_Header{
				HeaderName: "x-some-header",
			},
		},
	}
	r.Route.HashPolicy[1] = &envoy_api_v2_route.RouteAction_HashPolicy{
		PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
			Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
				Name: "nom-nom-nom",
			},
		},
		Terminal: true,
	}
	r.Route.HashPolicy[2] = &envoy_api_v2_route.RouteAction_HashPolicy{
		PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_ConnectionProperties_{
			ConnectionProperties: &envoy_api_v2_route.RouteAction_HashPolicy_ConnectionProperties{
				SourceIp: true,
			},
		},
	}

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("hashpolicy.hello.world",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: r,
			},
		),
	), nil)
}

func TestAdobeRoutePerFilterConfigAllowDeny(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "perfilterconfig-allow-deny.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				PerFilterConfig: &ingressroutev1.PerFilterConfig{
					IpAllowDeny: &ingressroutev1.IpAllowDenyCidrs{
						AllowCidrs: []ingressroutev1.Cidr{
							{
								AddressPrefix: func() *string { s := "10.0.0.1"; return &s }(),
								PrefixLen:     func() *intstr.IntOrString { i := intstr.FromInt(32); return &i }(),
							},
							{
								AddressPrefix: func() *string { s := "10.0.0.2"; return &s }(),
								PrefixLen:     func() *intstr.IntOrString { s := intstr.FromString("32"); return &s }(),
							},
						},
						DenyCidrs: []ingressroutev1.Cidr{
							{
								AddressPrefix: func() *string { s := "10.0.0.3"; return &s }(),
								PrefixLen:     func() *intstr.IntOrString { i := intstr.FromInt(32); return &i }(),
							},
						},
					},
				},
			}},
		},
	})

	r := &envoy_api_v2_route.Route{
		Match:  routePrefix("/"),
		Action: routecluster("default/ws/80/da39a3ee5e"),
	}
	r.TypedPerFilterConfig = map[string]*any.Any{
		"envoy.filters.http.ip_allow_deny": toAny(t, &_struct.Struct{
			Fields: map[string]*_struct.Value{
				"allow_cidrs": {
					Kind: &_struct.Value_ListValue{
						ListValue: &_struct.ListValue{
							Values: []*_struct.Value{
								{
									Kind: &_struct.Value_StructValue{
										StructValue: &_struct.Struct{
											Fields: map[string]*_struct.Value{
												"address_prefix": {
													Kind: &_struct.Value_StringValue{
														StringValue: "10.0.0.1",
													},
												},
												"prefix_len": {
													Kind: &_struct.Value_NumberValue{
														NumberValue: float64(32),
													},
												},
											},
										},
									},
								},
								{
									Kind: &_struct.Value_StructValue{
										StructValue: &_struct.Struct{
											Fields: map[string]*_struct.Value{
												"address_prefix": {
													Kind: &_struct.Value_StringValue{
														StringValue: "10.0.0.2",
													},
												},
												"prefix_len": {
													Kind: &_struct.Value_StringValue{
														StringValue: "32",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				"deny_cidrs": {
					Kind: &_struct.Value_ListValue{
						ListValue: &_struct.ListValue{
							Values: []*_struct.Value{
								{
									Kind: &_struct.Value_StructValue{
										StructValue: &_struct.Struct{
											Fields: map[string]*_struct.Value{
												"address_prefix": {
													Kind: &_struct.Value_StringValue{
														StringValue: "10.0.0.3",
													},
												},
												"prefix_len": {
													Kind: &_struct.Value_NumberValue{
														NumberValue: float64(32),
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}),
	}

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("perfilterconfig-allow-deny.hello.world", r),
	), nil)
}

func TestAdobeRoutePerFilterConfigHeaderSize(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "perfilterconfig-header-size.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				PerFilterConfig: &ingressroutev1.PerFilterConfig{
					HeaderSize: &ingressroutev1.HeaderSize{
						HeaderSize: struct {
							MaxBytes *int `json:"max_bytes,omitempty"`
						}{
							MaxBytes: func() *int { i := 8192; return &i }(),
						},
					},
				},
			}},
		},
	})

	r := &envoy_api_v2_route.Route{
		Match:  routePrefix("/"),
		Action: routecluster("default/ws/80/da39a3ee5e"),
	}
	r.TypedPerFilterConfig = map[string]*any.Any{
		"envoy.filters.http.header_size": toAny(t, &_struct.Struct{
			Fields: map[string]*_struct.Value{
				"header_size": {
					Kind: &_struct.Value_StructValue{
						StructValue: &_struct.Struct{
							Fields: map[string]*_struct.Value{
								"max_bytes": {
									Kind: &_struct.Value_NumberValue{
										NumberValue: float64(8192),
									},
								},
							},
						},
					},
				},
			},
		}),
	}

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("perfilterconfig-header-size.hello.world", r),
	), nil)
}

func TestAdobeRoutePerFilterConfigOrdered(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "perfilterconfig-ordered.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				PerFilterConfig: &ingressroutev1.PerFilterConfig{
					IpAllowDeny: &ingressroutev1.IpAllowDenyCidrs{ // unordered
						AllowCidrs: []ingressroutev1.Cidr{
							{
								AddressPrefix: func() *string { s := "10.0.0.1"; return &s }(),
								PrefixLen:     func() *intstr.IntOrString { i := intstr.FromInt(32); return &i }(),
							},
						},
					},
					HeaderSize: &ingressroutev1.HeaderSize{
						HeaderSize: struct {
							MaxBytes *int `json:"max_bytes,omitempty"`
						}{
							MaxBytes: func() *int { i := 8192; return &i }(),
						},
					},
				},
			}},
		},
	})

	r := &envoy_api_v2_route.Route{
		Match:  routePrefix("/"),
		Action: routecluster("default/ws/80/da39a3ee5e"),
	}
	r.TypedPerFilterConfig = map[string]*any.Any{
		"envoy.filters.http.header_size": toAny(t, &_struct.Struct{
			Fields: map[string]*_struct.Value{
				"header_size": {
					Kind: &_struct.Value_StructValue{
						StructValue: &_struct.Struct{
							Fields: map[string]*_struct.Value{
								"max_bytes": {
									Kind: &_struct.Value_NumberValue{
										NumberValue: float64(8192),
									},
								},
							},
						},
					},
				},
			},
		}),
		"envoy.filters.http.ip_allow_deny": toAny(t, &_struct.Struct{
			Fields: map[string]*_struct.Value{
				"allow_cidrs": {
					Kind: &_struct.Value_ListValue{
						ListValue: &_struct.ListValue{
							Values: []*_struct.Value{
								{
									Kind: &_struct.Value_StructValue{
										StructValue: &_struct.Struct{
											Fields: map[string]*_struct.Value{
												"address_prefix": {
													Kind: &_struct.Value_StringValue{
														StringValue: "10.0.0.1",
													},
												},
												"prefix_len": {
													Kind: &_struct.Value_NumberValue{
														NumberValue: float64(32),
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}),
	}

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("perfilterconfig-ordered.hello.world", r),
	), nil)
}

func TestAdobeRouteTimeout(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "route-timeout.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				Timeout: &ingressroutev1.Duration{
					duration.Duration{
						Seconds: int64(60),
						Nanos:   int32(0),
					},
				},
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	r.Route.Timeout = protobuf.Duration(60 * time.Second)

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("route-timeout.hello.world",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: r,
			},
		),
	), nil)
}

func TestAdobeRouteIdleTimeout(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "route-timeout.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				IdleTimeout: &ingressroutev1.Duration{
					duration.Duration{
						Seconds: int64(45),
						Nanos:   int32(0),
					},
				},
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	r.Route.IdleTimeout = protobuf.Duration(45 * time.Second)

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("route-timeout.hello.world",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: r,
			},
		),
	), nil)
}

func TestAdobeRouteServiceTimeout(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "service-timeout.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
					IdleTimeout: &ingressroutev1.Duration{
						duration.Duration{
							Seconds: int64(55),
							Nanos:   int32(0),
						},
					},
				}},
			}},
		},
	})

	c := cluster("default/ws/80/da39a3ee5e", "default/ws", "default_ws_80")
	c.CircuitBreakers = adobe.CircuitBreakers
	c.DrainConnectionsOnHostRemoval = true
	c.CommonHttpProtocolOptions = &envoy_api_v2_core.HttpProtocolOptions{
		IdleTimeout: protobuf.Duration(55 * time.Second),
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources:   resources(t, c),
		TypeUrl:     clusterType,
		Nonce:       "1",
	}, streamCDS(t, cc))
}

// ==== Hard-coded customization ====

// == internal/contour/listener.go
// remove stats listener
// add custom listeners (with CIDR_LIST_PATH)
// filter chain grouping

func TestAdobeListenerRemoveStats(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "no-stats-listener.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	protos := []proto.Message{
		&v2.Listener{
			Name:         "ingress_http",
			Address:      envoy.SocketAddress("0.0.0.0", 8080),
			FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
		},
		// staticListener(), // upstream expects this!
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc))
}

// TODO
func TestAdobeListenerCustomListeners(t *testing.T) {
	// TODO: configure the CIDR_LIST_PATH for all tests, ignore it instead of here?
	// rh, cc, done := setup(t)
	// defer done()
	//
	// // set up the env var
	// os.Setenv("CIDR_LIST_PATH", "ip_allow_deny.json")
	// defer os.Unsetenv("CIDR_LIST_PATH")
}

func TestAdobeListenerFilterChainGrouping(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// grouping is by secret, so create a single one
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "ns-secret",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

	// delegation
	rh.OnAdd(&ingressroutev1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "ns-secret",
		},
		Spec: ingressroutev1.TLSCertificateDelegationSpec{
			Delegations: []ingressroutev1.CertificateDelegation{{
				SecretName: "secret",
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	})

	// service 2 (so it's not in order)
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws2",
			Namespace: "ns2",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       81,
				TargetPort: intstr.FromInt(8081),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws2",
			Namespace: "ns2",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "fc-group-svc2.hello.world",
				TLS: &projcontour.TLS{
					SecretName: "ns-secret/secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws2",
					Port: 81,
				}},
			}},
		},
	})

	// service 1
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws1",
			Namespace: "ns1",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws1",
			Namespace: "ns1",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "fc-group-svc1.hello.world",
				TLS: &projcontour.TLS{
					SecretName: "ns-secret/secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws1",
					Port: 80,
				}},
			}},
		},
	})

	protos := []proto.Message{
		&v2.Listener{
			Name:         "ingress_http",
			Address:      envoy.SocketAddress("0.0.0.0", 8080),
			FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
		},
		&v2.Listener{
			Name:    "ingress_https",
			Address: envoy.SocketAddress("0.0.0.0", 8443),
			ListenerFilters: envoy.ListenerFilters(
				envoy.TLSInspector(),
			),
			FilterChains: []*envoy_api_v2_listener.FilterChain{
				{
					Filters: envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{
							"fc-group-svc1.hello.world",
							"fc-group-svc2.hello.world",
						},
					},
					TransportSocket: envoy.DownstreamTLSTransportSocket(envoy.DownstreamTLSContext(envoy.Secretname(&dag.Secret{Object: secret}), envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1")),
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "3",
	}, streamLDS(t, cc))
}

// A bug we introduced: don't group if the min TLS versions are different
// The fixture is virtually identical to the previous test, but for the
// min TLS protocol version on one of the ingress route
func TestAdobeListenerFilterChainGroupingNotTLS(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// grouping is by secret, so create a single one
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "ns-secret",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

	// delegation
	rh.OnAdd(&ingressroutev1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "ns-secret",
		},
		Spec: ingressroutev1.TLSCertificateDelegationSpec{
			Delegations: []ingressroutev1.CertificateDelegation{{
				SecretName: "secret",
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	})

	// service 2 (so it's not in order)
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws2",
			Namespace: "ns2",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       81,
				TargetPort: intstr.FromInt(8081),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws2",
			Namespace: "ns2",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "fc-group-svc2.hello.world",
				TLS: &projcontour.TLS{
					SecretName: "ns-secret/secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws2",
					Port: 81,
				}},
			}},
		},
	})

	// service 1
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws1",
			Namespace: "ns1",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws1",
			Namespace: "ns1",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "fc-group-svc1.hello.world",
				TLS: &projcontour.TLS{
					SecretName:             "ns-secret/secret",
					MinimumProtocolVersion: "1.3", // Different min version !!
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws1",
					Port: 80,
				}},
			}},
		},
	})

	protos := []proto.Message{
		&v2.Listener{
			Name:         "ingress_http",
			Address:      envoy.SocketAddress("0.0.0.0", 8080),
			FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
		},
		&v2.Listener{
			Name:    "ingress_https",
			Address: envoy.SocketAddress("0.0.0.0", 8443),
			ListenerFilters: envoy.ListenerFilters(
				envoy.TLSInspector(),
			),
			FilterChains: []*envoy_api_v2_listener.FilterChain{
				envoy.FilterChainTLS("fc-group-svc1.hello.world", &dag.Secret{Object: secret}, envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)), envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"),
				envoy.FilterChainTLS("fc-group-svc2.hello.world", &dag.Secret{Object: secret}, envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)), envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "3",
	}, streamLDS(t, cc))
}

// == internal/dag/builder.go
// re-allow root to root delegation
// disable TLS 1.1

// Test the whole scenario: delegation created in a separate ns with a different secret
func TestAdobeListenerRootToRootDelegation(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	secret1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret1)

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "roottoroot-root.hello.world",
				TLS: &projcontour.TLS{
					SecretName: "secret1",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	secret2 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret2",
			Namespace: "some-other-ns",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret2)

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegate",
			Namespace: "some-other-ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "roottoroot-dele.hello.world",
				TLS: &projcontour.TLS{
					SecretName: "secret2",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Delegate: &ingressroutev1.Delegate{
					Name:      "root",
					Namespace: "default",
				},
			}},
		},
	})

	protos := []proto.Message{
		&v2.Listener{
			Name:         "ingress_http",
			Address:      envoy.SocketAddress("0.0.0.0", 8080),
			FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
		},
		&v2.Listener{
			Name:    "ingress_https",
			Address: envoy.SocketAddress("0.0.0.0", 8443),
			ListenerFilters: envoy.ListenerFilters(
				envoy.TLSInspector(),
			),
			FilterChains: []*envoy_api_v2_listener.FilterChain{
				// "dele" first because of ordering
				envoy.FilterChainTLS("roottoroot-dele.hello.world", &dag.Secret{Object: secret2}, envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)), envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
				envoy.FilterChainTLS("roottoroot-root.hello.world", &dag.Secret{Object: secret1}, envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)), envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "2",
	}, streamLDS(t, cc))
}

func TestAdobeListenerDisableTLS1_1(t *testing.T) {
	// Done via Contour config
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.MinimumProtocolVersion = dag.MinProtoVersion("1.2")
	})
	defer done()

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "ns",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "ns",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8081),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "no-tls-1-1.hello.world",
				TLS: &projcontour.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "1.1",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	protos := []proto.Message{
		&v2.Listener{
			Name:         "ingress_http",
			Address:      envoy.SocketAddress("0.0.0.0", 8080),
			FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
		},
		&v2.Listener{
			Name:    "ingress_https",
			Address: envoy.SocketAddress("0.0.0.0", 8443),
			ListenerFilters: envoy.ListenerFilters(
				envoy.TLSInspector(),
			),
			FilterChains: []*envoy_api_v2_listener.FilterChain{
				envoy.FilterChainTLS("no-tls-1-1.hello.world", &dag.Secret{Object: secret}, envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)), envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc))
}

// == internal/dag/cache.go
// enforce class annotation
// TODO(lrouquet) - hard to test, easy to verify: skipping for now

// == internal/envoy/cluster.go
// add CircuitBreakers
// add DrainConnectionsOnHostRemoval
// Cluster_LbPolicy changes: remove Cookie/replace with RingHash, add MagLev
// CommonLbConfig: enable HealthyPanicThreshold

// test both at the same time since they're both added
func TestAdobeClusterCircuitBreakersDrainConnections(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "circuitbreaker-drain.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	c := cluster("default/ws/80/da39a3ee5e", "default/ws", "default_ws_80")
	c.CircuitBreakers = adobe.CircuitBreakers
	c.DrainConnectionsOnHostRemoval = true
	c.CommonHttpProtocolOptions = adobe.CommonHttpProtocolOptions

	protos := []proto.Message{c}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     clusterType,
		Nonce:       "1",
	}, streamCDS(t, cc))
}

// Cookie should now be default (so round robin)
// RingHash is RingHash (was cookie)
// Maglev is new
func TestAdobeClusterLbPolicy(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws-cookie",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws-ringhash",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws-maglev",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "lb-policy.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{
					{Name: "ws-cookie", Port: 80, Strategy: "Cookie"},
					{Name: "ws-ringhash", Port: 80, Strategy: "RingHash"},
					{Name: "ws-maglev", Port: 80, Strategy: "Maglev"},
				},
			}},
		},
	})

	cCookie := cluster("default/ws-cookie/80/e4f81994fe", "default/ws-cookie", "default_ws-cookie_80")
	cCookie.CircuitBreakers = adobe.CircuitBreakers
	cCookie.DrainConnectionsOnHostRemoval = true
	cCookie.LbPolicy = v2.Cluster_ROUND_ROBIN
	cCookie.CommonHttpProtocolOptions = adobe.CommonHttpProtocolOptions

	cRingHash := cluster("default/ws-ringhash/80/40633a6ca9", "default/ws-ringhash", "default_ws-ringhash_80")
	cRingHash.CircuitBreakers = adobe.CircuitBreakers
	cRingHash.DrainConnectionsOnHostRemoval = true
	cRingHash.LbPolicy = v2.Cluster_RING_HASH
	cRingHash.CommonHttpProtocolOptions = adobe.CommonHttpProtocolOptions

	cMagLev := cluster("default/ws-maglev/80/843e4ded8f", "default/ws-maglev", "default_ws-maglev_80")
	cMagLev.CircuitBreakers = adobe.CircuitBreakers
	cMagLev.DrainConnectionsOnHostRemoval = true
	cMagLev.LbPolicy = v2.Cluster_MAGLEV
	cMagLev.CommonHttpProtocolOptions = adobe.CommonHttpProtocolOptions

	protos := []proto.Message{cCookie, cMagLev, cRingHash} //ordered

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     clusterType,
		Nonce:       "1",
	}, streamCDS(t, cc))
}

// HealthyPanicThreshold=100
func TestAdobeClusterHealthyPanicThreshold(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "panic.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	c := &v2.Cluster{
		Name:                 "default/ws/80/da39a3ee5e",
		AltStatName:          "default_ws_80",
		ClusterDiscoveryType: envoy.ClusterDiscoveryType(v2.Cluster_EDS),
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig:   envoy.ConfigSource("contour"),
			ServiceName: "default/ws",
		},
		ConnectTimeout:                protobuf.Duration(250 * time.Millisecond),
		CircuitBreakers:               adobe.CircuitBreakers,
		DrainConnectionsOnHostRemoval: true,
		CommonHttpProtocolOptions:     adobe.CommonHttpProtocolOptions,

		// HealthyPanicThreshold
		CommonLbConfig: &v2.Cluster_CommonLbConfig{
			HealthyPanicThreshold: &envoy_type.Percent{
				Value: 100,
			},
		},
	}

	protos := []proto.Message{c}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     clusterType,
		Nonce:       "1",
	}, streamCDS(t, cc))
}

// == internal/envoy/healthcheck
// add ExpectedStatuses

func TestAdobeClusterHealthcheck(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "healthcheck.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
					HealthCheck: &ingressroutev1.HealthCheck{
						Path: "/health",
					},
				}},
			}},
		},
	})

	c := cluster("default/ws/80/0bb4da595b", "default/ws", "default_ws_80")
	c.CircuitBreakers = adobe.CircuitBreakers
	c.DrainConnectionsOnHostRemoval = true
	c.HealthChecks = []*envoy_api_v2_core.HealthCheck{
		{
			Timeout:            protobuf.Duration(2 * time.Second),
			Interval:           protobuf.Duration(10 * time.Second),
			UnhealthyThreshold: protobuf.UInt32(3),
			HealthyThreshold:   protobuf.UInt32(2),
			HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
				HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
					Path:             "/health",
					Host:             "contour-envoy-healthcheck",
					ExpectedStatuses: adobe.ExpectedStatuses,
				},
			},
		},
	}
	c.CommonHttpProtocolOptions = adobe.CommonHttpProtocolOptions

	protos := []proto.Message{c}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     clusterType,
		Nonce:       "1",
	}, streamCDS(t, cc))
}

// == internal/envoy/listener.go
// add SocketOptions
// HttpFilters updates:  remove gzip, grpc web, add ip_allow_deny, health_check_simple, headersize
// add GenerateRequestId:   protobuf.Bool(false),
// add MaxRequestHeadersKb: protobuf.UInt32(64),
// remove IdleTimeout
// remove PreserveExternalRequestId
// add tracing
// add MergeSlashes

func TestAdobeListenerSocketOptions(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// set up the env var
	os.Setenv("TCP_KEEPALIVE_ENABLED", "true")
	os.Setenv("TCP_KEEPALIVE_IDLE", "35")
	os.Setenv("TCP_KEEPALIVE_INTVL", "45")
	os.Setenv("TCP_KEEPALIVE_CNT", "55")

	defer func() {
		os.Unsetenv("TCP_KEEPALIVE_ENABLED")
		os.Unsetenv("TCP_KEEPALIVE_IDLE")
		os.Unsetenv("TCP_KEEPALIVE_INTVL")
		os.Unsetenv("TCP_KEEPALIVE_CNT")
	}()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "socket-options.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	protos := []proto.Message{
		&v2.Listener{
			Name:         "ingress_http",
			Address:      envoy.SocketAddress("0.0.0.0", 8080),
			FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
			SocketOptions: []*envoy_api_v2_core.SocketOption{
				{
					Level: 1,                                                     // SOL_SOCKET
					Name:  9,                                                     // SO_KEEPALIVE
					Value: &envoy_api_v2_core.SocketOption_IntValue{IntValue: 1}, // TCP_KEEPALIVE_ENABLED
					State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
				},
				{
					Level: 6,                                                      // IPPROTO_TCP
					Name:  4,                                                      // TCP_KEEPIDLE
					Value: &envoy_api_v2_core.SocketOption_IntValue{IntValue: 35}, // TCP_KEEPALIVE_IDLE
					State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
				},
				{
					Level: 6,                                                      // IPPROTO_TCP
					Name:  5,                                                      // TCP_KEEPINTVL
					Value: &envoy_api_v2_core.SocketOption_IntValue{IntValue: 45}, // TCP_KEEPALIVE_INTVL
					State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
				},
				{
					Level: 6,                                                      // IPPROTO_TCP
					Name:  6,                                                      // TCP_KEEPCNT
					Value: &envoy_api_v2_core.SocketOption_IntValue{IntValue: 55}, // TCP_KEEPALIVE_CNT
					State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc))
}

// test all of the rest in 1 swoop
func TestAdobeListenerHttpConnectionManager(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// set up the env var for tracing
	os.Setenv("TRACING_ENABLED", "true")
	os.Setenv("TRACING_OPERATION_NAME", "egress")
	os.Setenv("TRACING_CLIENT_SAMPLING", "25")
	os.Setenv("TRACING_RANDOM_SAMPLING", "35")
	os.Setenv("TRACING_OVERALL_SAMPLING", "45")
	os.Setenv("TRACING_VERBOSE", "false")

	defer func() {
		os.Unsetenv("TRACING_ENABLED")
		os.Unsetenv("TRACING_OPERATION_NAME")
		os.Unsetenv("TRACING_CLIENT_SAMPLING")
		os.Unsetenv("TRACING_RANDOM_SAMPLING")
		os.Unsetenv("TRACING_OVERALL_SAMPLING")
		os.Unsetenv("TRACING_VERBOSE")
	}()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "no-stats-listener.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	protos := []proto.Message{
		&v2.Listener{
			Name:    "ingress_http",
			Address: envoy.SocketAddress("0.0.0.0", 8080),
			FilterChains: []*envoy_api_v2_listener.FilterChain{
				{
					Filters: []*envoy_api_v2_listener.Filter{
						{
							Name: "envoy.http_connection_manager",
							ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
								TypedConfig: toAny(t, &http.HttpConnectionManager{
									StatPrefix: "ingress_http",
									RouteSpecifier: &http.HttpConnectionManager_Rds{
										Rds: &http.Rds{
											RouteConfigName: "ingress_http",
											ConfigSource: &envoy_api_v2_core.ConfigSource{
												ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
													ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
														ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
														GrpcServices: []*envoy_api_v2_core.GrpcService{{
															TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
																EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
																	ClusterName: "contour",
																},
															},
														}},
													},
												},
											},
										},
									},
									GenerateRequestId:   protobuf.Bool(false),
									MaxRequestHeadersKb: protobuf.UInt32(64),
									HttpFilters: []*http.HttpFilter{
										{
											Name: "envoy.filters.http.ip_allow_deny",
										},
										{
											Name: "envoy.filters.http.health_check_simple",
											ConfigType: &http.HttpFilter_TypedConfig{
												TypedConfig: toAny(t, &udpa_type_v1.TypedStruct{
													TypeUrl: "envoy.config.filter.http.health_check_simple.v2.HealthCheckSimple",
													Value: &_struct.Struct{
														Fields: map[string]*_struct.Value{
															"path": {Kind: &_struct.Value_StringValue{StringValue: "/envoy_health_94eaa5a6ba44fc17d1da432d4a1e2d73"}},
														},
													},
												}),
											},
										},
										{
											Name: "envoy.filters.http.header_size",
											ConfigType: &http.HttpFilter_TypedConfig{
												TypedConfig: toAny(t, &udpa_type_v1.TypedStruct{
													TypeUrl: "envoy.config.filter.http.header_size.v2.HeaderSize",
													Value: &_struct.Struct{
														Fields: map[string]*_struct.Value{
															"max_bytes": {Kind: &_struct.Value_NumberValue{NumberValue: 64 * 1024}},
														},
													},
												}),
											},
										},
										{
											Name: "envoy.router",
										},
									},
									HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
										AcceptHttp_10: true,
									},
									AccessLog:        envoy.FileAccessLogEnvoy("/dev/stdout"),
									UseRemoteAddress: protobuf.Bool(true),
									NormalizePath:    protobuf.Bool(true),
									RequestTimeout:   protobuf.Duration(0),
									MergeSlashes:     true,
									Tracing: &http.HttpConnectionManager_Tracing{
										OperationName:   http.HttpConnectionManager_Tracing_EGRESS,
										ClientSampling:  &envoy_type.Percent{Value: 25},
										RandomSampling:  &envoy_type.Percent{Value: 35},
										OverallSampling: &envoy_type.Percent{Value: 45},
										Verbose:         false,
									},
								}),
							},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc))
}

// == internal/envoy/route.go
// remove RouteAction.RetryPolicy
// remove RouteHeader "x-request-start"
// add VirtualHost.RetryPolicy

func TestAdobeRoute(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "route.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	protos := []proto.Message{
		&v2.RouteConfiguration{
			Name: "ingress_http",
			VirtualHosts: []*envoy_api_v2_route.VirtualHost{
				{
					Name: "route.hello.world",
					Domains: []string{
						"route.hello.world",
						"route.hello.world:*",
					},
					Routes: []*envoy_api_v2_route.Route{
						{
							Match:  routePrefix("/"),
							Action: routecluster("default/ws/80/da39a3ee5e"),
						},
					},
					RetryPolicy: &envoy_api_v2_route.RetryPolicy{
						RetryOn:                       "connect-failure",
						NumRetries:                    protobuf.UInt32(3),
						HostSelectionRetryMaxAttempts: 3,
					},
				},
			},
		},
		&v2.RouteConfiguration{
			Name:         "ingress_https",
			VirtualHosts: nil,
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}
