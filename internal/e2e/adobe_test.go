// All tests func should start with TestAdobe (fixup is applied only to
// non-adobe tests; the name is used to make that determination)
//
// tests are organized by their name: TestAdobe{Cluster|Listener|Route}...
// this is so we can filter via "--run TestAdobeCluster" etc.

package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	udpa_type_v1 "github.com/cncf/udpa/go/udpa/type/v1"
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	router "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/router/v2"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/adobe"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ==== IngressRoute CRD customization ====
// == Route
// HashPolicy []HashPolicy `json:"hashPolicy,omitempty"`
// PerFilterConfig *PerFilterConfig `json:"perFilterConfig,omitempty"`
// TimeoutPolicy doesn't override Timeout
// Timeout *Duration `json:"timeout,omitempty"`
// IdleTimeout *Duration `json:"idleTimeout,omitempty"`
// Tracing *Tracing `json:"tracing,omitempty"`
// RequestHeadersPolicy *HeadersPolicy `json:"requestHeadersPolicy,omitempty"`
// ResponseHeadersPolicy *HeadersPolicy `json:"responseHeadersPolicy,omitempty"`
// HeaderMatch []projcontour.HeaderCondition `json:"headerMatch,omitempty"`
// EnableSPDY bool `json:"enableSPDY,omitempty"`
//
// == Service
// IdleTimeout *Duration `json:"idleTimeout,omitempty"`
//
// == TLS
// MaximumProtocolVersion string `json:"maximumProtocolVersion,omitempty"`
//
// == Annotations
// Support 'adobeplatform.adobe.io/hosts: foo.bar.adobe.com'

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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "hashpolicy.hello.world"},
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "perfilterconfig-allow-deny.hello.world"},
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
		"envoy.filters.http.ip_allow_deny": protobuf.MustMarshalAny(&_struct.Struct{
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "perfilterconfig-header-size.hello.world"},
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
		"envoy.filters.http.header_size": protobuf.MustMarshalAny(&_struct.Struct{
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "perfilterconfig-ordered.hello.world"},
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
		"envoy.filters.http.header_size": protobuf.MustMarshalAny(&_struct.Struct{
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
		"envoy.filters.http.ip_allow_deny": protobuf.MustMarshalAny(&_struct.Struct{
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

func TestAdobeRouteTimeoutPolicy(t *testing.T) {
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route-timeoutpolicy.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				Timeout: &ingressroutev1.Duration{
					duration.Duration{
						Seconds: int64(11),
						Nanos:   int32(0),
					},
				},
				TimeoutPolicy: &ingressroutev1.TimeoutPolicy{
					Request: "12s",
				},
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	r.Route.Timeout = protobuf.Duration(11 * time.Second) // Timeout overrides TimeoutPolicy

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("route-timeoutpolicy.hello.world",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: r,
			},
		),
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route-timeout.hello.world"},
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

func TestAdobeRouteTracing(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	os.Setenv("TRACING_ENABLED", "true")
	defer func() {
		os.Unsetenv("TRACING_ENABLED")
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "tracing.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				Tracing: &ingressroutev1.Tracing{
					ClientSampling: uint8(65),
					RandomSampling: uint8(75),
				},
			}},
		},
	})

	r := &envoy_api_v2_route.Route{
		Match:  routePrefix("/"),
		Action: routecluster("default/ws/80/da39a3ee5e"),
	}
	r.Tracing = &envoy_api_v2_route.Tracing{
		ClientSampling: &envoy_type.FractionalPercent{Numerator: uint32(65)},
		RandomSampling: &envoy_type.FractionalPercent{Numerator: uint32(75)},
	}

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("tracing.hello.world", r),
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route-timeout.hello.world"},
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "service-timeout.hello.world"},
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

func TestAdobeRouteHeaderMatch(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws1",
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
			Name:      "ws2",
			Namespace: "default",
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
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route-headermatch.hello.world"},
			Routes: []ingressroutev1.Route{
				{
					Match: "/",
					HeaderMatch: []projcontour.HeaderCondition{{
						Name:  "my-header",
						Exact: "foo",
					}},
					Services: []ingressroutev1.Service{{
						Name: "ws1",
						Port: 80,
					}},
				},
				{
					Match: "/",
					HeaderMatch: []projcontour.HeaderCondition{{
						Name:  "my-header",
						Exact: "bar",
					}},
					Services: []ingressroutev1.Service{{
						Name: "ws2",
						Port: 81,
					}},
				},
			},
		},
	})

	r1 := &envoy_api_v2_route.Route{
		Match: &envoy_api_v2_route.RouteMatch{
			PathSpecifier: &envoy_api_v2_route.RouteMatch_Prefix{
				Prefix: "/",
			},
			Headers: []*envoy_api_v2_route.HeaderMatcher{{
				Name:                 "my-header",
				HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_ExactMatch{ExactMatch: "foo"},
			}},
		},
		Action: routecluster("default/ws1/80/da39a3ee5e"),
	}

	r2 := &envoy_api_v2_route.Route{
		Match: &envoy_api_v2_route.RouteMatch{
			PathSpecifier: &envoy_api_v2_route.RouteMatch_Prefix{
				Prefix: "/",
			},
			Headers: []*envoy_api_v2_route.HeaderMatcher{{
				Name:                 "my-header",
				HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_ExactMatch{ExactMatch: "bar"},
			}},
		},
		Action: routecluster("default/ws2/81/da39a3ee5e"),
	}

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("route-headermatch.hello.world", r2, r1), // matches are ordered
	), nil)
}

func TestAdobeRouteEnableSPDY(t *testing.T) {
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route-spdy.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				EnableSPDY: true,
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	r.Route.UpgradeConfigs = []*envoy_api_v2_route.RouteAction_UpgradeConfig{
		{
			UpgradeType: "spdy/3.1",
		},
	}

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("route-spdy.hello.world",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: r,
			},
		),
	), nil)
}

func TestAdobeTLSMaximumProtocolVersion(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "tls-max-version.hello.world",
				TLS: &ingressroutev1.TLS{
					SecretName:             "secret",
					MaximumProtocolVersion: "1.2",
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

	tlsContext := envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1")
	tlsContext.CommonTlsContext.TlsParams.TlsMaximumProtocolVersion = envoy_api_v2_auth.TlsParameters_TLSv1_2

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
							"tls-max-version.hello.world",
						},
					},
					TransportSocket: envoy.DownstreamTLSTransportSocket(tlsContext),
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

func TestAdobeAnnotationHosts(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// should work for both HTTP and HTTPS
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

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
			Annotations: map[string]string{
				"adobeplatform.adobe.io/hosts": "foo.bar.adobe.com",
			},
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "annotation-host.hello.world",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match:          "/",
				PermitInsecure: true,
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	vhosts := []*envoy_api_v2_route.VirtualHost{
		{
			Name: "annotation-host.hello.world",
			Domains: []string{
				"annotation-host.hello.world",
				"annotation-host.hello.world:*",
				"foo.bar.adobe.com",
			},
			Routes: []*envoy_api_v2_route.Route{
				{
					Match:  routePrefix("/"),
					Action: r,
				},
			},
			RetryPolicy: adobe.RetryPolicy,
		},
	}

	protos := []proto.Message{
		&v2.RouteConfiguration{
			Name:         "ingress_http",
			VirtualHosts: vhosts,
		},
		&v2.RouteConfiguration{
			Name:         "ingress_https",
			VirtualHosts: vhosts,
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}

func TestAdobeAnnotationHostsWithDelegation(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// should work for both HTTP and HTTPS
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

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
			Name:      "simple-base",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match:          "/",
				PermitInsecure: true,
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"adobeplatform.adobe.io/hosts": "foo.bar.adobe.com",
			},
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "annotation-host-dele.hello.world",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Delegate: &ingressroutev1.Delegate{
					Name:      "simple-base",
					Namespace: "default",
				},
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	vhosts := []*envoy_api_v2_route.VirtualHost{
		{
			Name: "annotation-host-dele.hello.world",
			Domains: []string{
				"annotation-host-dele.hello.world",
				"annotation-host-dele.hello.world:*",
				"foo.bar.adobe.com",
			},
			Routes: []*envoy_api_v2_route.Route{
				{
					Match:  routePrefix("/"),
					Action: r,
				},
			},
			RetryPolicy: adobe.RetryPolicy,
		},
	}

	protos := []proto.Message{
		&v2.RouteConfiguration{
			Name:         "ingress_http",
			VirtualHosts: vhosts,
		},
		&v2.RouteConfiguration{
			Name:         "ingress_https",
			VirtualHosts: vhosts,
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}

// == internal/envoy/route.go
// add Route.RequestHeadersPolicy
// add Route.ResponseHeadersPolicy

func TestAdobeRouteHeaderRewritePolicy(t *testing.T) {
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route-header-rewrite.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				RequestHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name:  "In-Foo",
						Value: "bar",
					},
						{
							Name:  "Request-Header-Two-Insert",
							Value: "InFooBarTheSecond",
						}},
					Remove: []string{
						"Abbracadabra",
						"In-Bar",
						"In-Baz",
					},
				},
				ResponseHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name:  "Out-Foo",
						Value: "goodbye",
					},
						{
							Name:  "Response-Header-Two-Insert",
							Value: "OutFooBarTheSecond",
						}},
					Remove: []string{
						"Out-Baz",
						"Request-Header-Two-Insert",
					},
				},
			}},
		},
	})

	protos := []proto.Message{
		&v2.RouteConfiguration{
			Name: "ingress_http",
			VirtualHosts: []*envoy_api_v2_route.VirtualHost{
				{
					Name: "route-header-rewrite.hello.world",
					Domains: []string{
						"route-header-rewrite.hello.world",
						"route-header-rewrite.hello.world:*",
					},
					Routes: []*envoy_api_v2_route.Route{
						{
							Match:  routePrefix("/"),
							Action: routecluster("default/ws/80/da39a3ee5e"),
							RequestHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{
								{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "In-Foo",
										Value: "bar",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								},
								{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "Request-Header-Two-Insert",
										Value: "InFooBarTheSecond",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								},
							},
							RequestHeadersToRemove: []string{
								"Abbracadabra",
								"In-Bar",
								"In-Baz",
							},
							ResponseHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{
								{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "Out-Foo",
										Value: "goodbye",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								},
								{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "Response-Header-Two-Insert",
										Value: "OutFooBarTheSecond",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								},
							},
							ResponseHeadersToRemove: []string{
								"Out-Baz",
								"Request-Header-Two-Insert",
							},
						},
					},
					RetryPolicy: &envoy_api_v2_route.RetryPolicy{
						RetryOn:                       "reset",
						NumRetries:                    protobuf.UInt32(3),
						HostSelectionRetryMaxAttempts: 3,
						RetryHostPredicate: []*envoy_api_v2_route.RetryPolicy_RetryHostPredicate{{
							Name: "envoy.retry_host_predicates.previous_hosts",
						}},
					},
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}

// ==== Hard-coded customization ====

// == internal/contour/listener.go
// remove stats listener
// add custom listeners (with CIDR_LIST_PATH)
// default TLS server (with DEFAULT_CERTIFICATE)
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "no-stats-listener.hello.world"},
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

func TestAdobeListenerDefaultTLSServer(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.DefaultCertificate = "ns/secret"
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

	// we need at least 1 cluster to trigger a listener build
	// in the real world, envoy won't request LDS if CDS is empty
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "ns",
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
			Namespace: "ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "default.hello.world",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
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
				envoy.FilterChainTLS("", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
				envoy.FilterChainTLS("default.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-svc2.hello.world",
				TLS: &ingressroutev1.TLS{
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-svc1.hello.world",
				TLS: &ingressroutev1.TLS{
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
					TransportSocket: envoy.DownstreamTLSTransportSocket(envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1")),
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-svc2.hello.world",
				TLS: &ingressroutev1.TLS{
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-svc1.hello.world",
				TLS: &ingressroutev1.TLS{
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
				envoy.FilterChainTLS("fc-group-svc1.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
				envoy.FilterChainTLS("fc-group-svc2.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
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

// Another bug: don't group http and tcp_proxy services
// the issue occurs when the http service is merged -into- an existing
// filterchain for a tcp service
func TestAdobeListenerFilterChainGroupingNotTCPProxy(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// grouping is by secret, so create a single one
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "ns",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

	// service 1 - http
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http",
			Namespace: "ns",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     81,
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http",
			Namespace: "ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-2http.hello.world", // 2 for ordering
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "http",
					Port: 81,
				}},
			}},
		},
	})

	// service 2 - tcp
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp",
			Namespace: "ns",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp",
			Namespace: "ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-1tcp.hello.world", // 1 for ordering
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: "tcp",
					Port: 80,
				}},
			},
		},
	})

	// hold the cluster for the tcp service
	tcpCluster := &dag.Cluster{
		Upstream: &dag.Service{
			Name:      "tcp",
			Namespace: "ns",
			ServicePort: &v1.ServicePort{
				Protocol: "TCP",
				Port:     80,
			},
		},
	}

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
				envoy.FilterChainTLS("fc-group-1tcp.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil), envoy.Filters(envoy.TCPProxy("ingress_https", &dag.TCPProxy{Clusters: []*dag.Cluster{tcpCluster}}, envoy.FileAccessLogEnvoy("/dev/stdout")))),
				envoy.FilterChainTLS("fc-group-2http.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
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

// One more: grouping stops if an existing group with a tcpproxy filter is found
// e.g. 2 groups exists: tcp and http, another http service than can be grouped
// with the 1st one wasn't
func TestAdobeListenerFilterChainGroupingTCPProxyStopsIt(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// grouping is by secret, so create a single one
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "ns",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

	// service 1 - http
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http1",
			Namespace: "ns",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     81,
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http1",
			Namespace: "ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-2http1.hello.world", // 2 for ordering
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "http1",
					Port: 81,
				}},
			}},
		},
	})

	// service 2 - tcp
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp",
			Namespace: "ns",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp",
			Namespace: "ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-1tcp.hello.world", // 1 for ordering
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: "tcp",
					Port: 80,
				}},
			},
		},
	})

	// service 2 - http
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http2",
			Namespace: "ns",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     81,
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http2",
			Namespace: "ns",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "fc-group-3http2.hello.world", // 3 for ordering
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "http2",
					Port: 81,
				}},
			}},
		},
	})

	// hold the cluster for the tcp service
	tcpCluster := &dag.Cluster{
		Upstream: &dag.Service{
			Name:      "tcp",
			Namespace: "ns",
			ServicePort: &v1.ServicePort{
				Protocol: "TCP",
				Port:     80,
			},
		},
	}

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
				envoy.FilterChainTLS("fc-group-1tcp.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil), envoy.Filters(envoy.TCPProxy("ingress_https", &dag.TCPProxy{Clusters: []*dag.Cluster{tcpCluster}}, envoy.FileAccessLogEnvoy("/dev/stdout")))),
				{
					Filters: envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{
							"fc-group-2http1.hello.world",
							"fc-group-3http2.hello.world",
						},
					},
					TransportSocket: envoy.DownstreamTLSTransportSocket(envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1")),
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

// == internal/dag/builder.go
// re-allow root to root delegation
// disable TLS 1.1
// allow wildcard fqdn (incl. '*')
// allow dynamic request/response headers

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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "roottoroot-root.hello.world",
				TLS: &ingressroutev1.TLS{
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "roottoroot-dele.hello.world",
				TLS: &ingressroutev1.TLS{
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
				envoy.FilterChainTLS("roottoroot-dele.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret2}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
				envoy.FilterChainTLS("roottoroot-root.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret1}, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
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
		reh.CacheHandler.MinimumProtocolVersion = annotation.MinProtoVersion("1.2")
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "no-tls-1-1.hello.world",
				TLS: &ingressroutev1.TLS{
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
				envoy.FilterChainTLS("no-tls-1-1.hello.world", envoy.DownstreamTLSContext(&dag.Secret{Object: secret}, envoy_api_v2_auth.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"), envoy.Filters(envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0))),
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

func TestAdobeListenerWildcardFqdn(t *testing.T) {
	rh, cc, done := setup(t)
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "*.hello.world.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
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
			FilterChains: filterchaintls("*.hello.world.com", secret, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc))
}

func TestAdobeListenerStarFqdn(t *testing.T) {
	rh, cc, done := setup(t)
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "*",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
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
			FilterChains: filterchaintls("*", secret, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc))
}

func TestAdobeRouteDynamicCustomHeader(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "dynamic-header.hello.world.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				RequestHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{
						{
							Name:  "x-forwarded-tls-cipher",
							Value: "__percent__DOWNSTREAM_TLS_CIPHER__percent__",
						},
					},
				},
				ResponseHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{
						{
							Name:  "x-envoy-upstream-remote-address",
							Value: "__percent__UPSTREAM_REMOTE_ADDRESS__percent__",
						},
					},
				},
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
					Name: "dynamic-header.hello.world.com",
					Domains: []string{
						"dynamic-header.hello.world.com",
						"dynamic-header.hello.world.com:*",
					},
					Routes: []*envoy_api_v2_route.Route{
						{
							Match:  routePrefix("/"),
							Action: routecluster("ns/ws/80/da39a3ee5e"),
							RequestHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{
								{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "X-Forwarded-Tls-Cipher",
										Value: "%DOWNSTREAM_TLS_CIPHER%",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								},
							},
							ResponseHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{
								{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "X-Envoy-Upstream-Remote-Address",
										Value: "%UPSTREAM_REMOTE_ADDRESS%",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								},
							},
						},
					},
					RetryPolicy: adobe.RetryPolicy,
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}

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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "circuitbreaker-drain.hello.world"},
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "lb-policy.hello.world"},
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

// HealthyPanicThreshold=10
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "panic.hello.world"},
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
				Value: 10,
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
// add InitialJitter = 1s
// add IntervalJitterPercent = 100
// optional logging

func TestAdobeClusterHealthcheck(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// set up the env var
	os.Setenv("HC_FAILURE_LOGGING_ENABLED", "true")

	defer func() {
		os.Unsetenv("HC_FAILURE_LOGGING_ENABLED")
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "healthcheck.hello.world"},
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
			Timeout:               protobuf.Duration(2 * time.Second),
			Interval:              protobuf.Duration(10 * time.Second),
			InitialJitter:         protobuf.Duration(1 * time.Second),
			IntervalJitterPercent: 100,
			UnhealthyThreshold:    protobuf.UInt32(3),
			HealthyThreshold:      protobuf.UInt32(2),
			HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
				HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
					Path:             "/health",
					Host:             "contour-envoy-healthcheck",
					ExpectedStatuses: adobe.ExpectedStatuses,
				},
			},
			EventLogPath:                 "/dev/stderr",
			AlwaysLogHealthCheckFailures: true,
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
// HttpFilters changes:  router: set SuppressEnvoyHeaders
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "socket-options.hello.world"},
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "no-stats-listener.hello.world"},
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
								TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
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
												TypedConfig: protobuf.MustMarshalAny(&udpa_type_v1.TypedStruct{
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
												TypedConfig: protobuf.MustMarshalAny(&udpa_type_v1.TypedStruct{
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
											ConfigType: &http.HttpFilter_TypedConfig{
												TypedConfig: protobuf.MustMarshalAny(&router.Router{
													SuppressEnvoyHeaders: true,
												}),
											},
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
									ServerName:       "adobe",
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
// merge RouteAction.RetryPolicy
// remove RouteHeader "x-request-start"
// add VirtualHost.RetryPolicy
// ensure Ingress.Annotation["contour.heptio.com/response-timeout"] can't be < 0

// test the RetryPolicy merging on its own
func TestAdobeRouteRetryPolicy(t *testing.T) {
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
				RetryPolicy: &projcontour.RetryPolicy{
					NumRetries:    51,
					PerTryTimeout: "123s",
				},
			}},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	r.Route.RetryPolicy = &envoy_api_v2_route.RetryPolicy{
		RetryOn:                       strings.Join([]string{"5xx", adobe.RetryPolicy.RetryOn}, ","),
		NumRetries:                    protobuf.UInt32(51),
		PerTryTimeout:                 protobuf.Duration(123 * time.Second),
		HostSelectionRetryMaxAttempts: adobe.RetryPolicy.HostSelectionRetryMaxAttempts,
	}

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
							Action: r,
						},
					},
					RetryPolicy: adobe.RetryPolicy,
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}

func TestAdobeIngressAnnotationTimeout(t *testing.T) {
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

	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/response-timeout": "-1s", // negative
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "ws",
				ServicePort: intstr.FromInt(80),
			},
		},
	})

	r := routecluster("default/ws/80/da39a3ee5e")
	r.Route.Timeout = protobuf.Duration(0) // contour floors this to 0

	protos := []proto.Message{
		&v2.RouteConfiguration{
			Name: "ingress_http",
			VirtualHosts: []*envoy_api_v2_route.VirtualHost{
				{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*envoy_api_v2_route.Route{
						{
							Match:  routePrefix("/"),
							Action: r,
						},
					},
					RetryPolicy: adobe.RetryPolicy,
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}

// test all of the rest in 1 swoop
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "route.hello.world"},
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
					RetryPolicy: adobe.RetryPolicy,
				},
			},
		},
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: adobe.Hash(protos),
		Resources:   resources(t, protos...),
		TypeUrl:     routeType,
		Nonce:       "1",
	}, streamRDS(t, cc))
}

// == internal/contour/endpointstranslator.go
// sort endpoints
func TestAdobeSortEndpoints(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	e1 := endpoints(
		"test_ns",
		"test_cluster",
		v1.EndpointSubset{
			Addresses: addresses(
				"172.16.195.1",
				"172.16.195.2",
				"172.16.196.1",
				"172.16.196.2",
			),
			Ports: ports(
				port("port", 3000),
			),
		},
	)

	rh.OnAdd(e1)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			envoy.ClusterLoadAssignment(
				"test_ns/test_cluster/port",
				envoy.SocketAddress("172.16.195.1", 3000),
				envoy.SocketAddress("172.16.196.1", 3000),
				envoy.SocketAddress("172.16.195.2", 3000),
				envoy.SocketAddress("172.16.196.2", 3000),
			),
		),
		TypeUrl: endpointType,
		Nonce:   "2",
	}, streamEDS(t, cc))
}
