// Copyright © 2019 VMware
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envoy

import (
	"testing"
	"time"

	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRoute(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	cluster := &dag.Cluster{
		Upstream: &dag.Service{
			Name:        service.Name,
			Namespace:   service.Namespace,
			ServicePort: &service.Spec.Ports[0],
		},
	}
	match := RoutePrefix("/")
	action := RouteRoute(&dag.Route{
		Clusters: []*dag.Cluster{cluster},
	})
	got := Route(match, action)
	want := &envoy_api_v2_route.Route{
		Match:               match,
		Action:              action,
		RequestHeadersToAdd: RouteHeaders(),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestRouteRoute(t *testing.T) {
	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	c1 := &dag.Cluster{
		Upstream: &dag.Service{
			Name:        s1.Name,
			Namespace:   s1.Namespace,
			ServicePort: &s1.Spec.Ports[0],
		},
	}
	c2 := &dag.Cluster{
		Upstream: &dag.Service{
			Name:        s1.Name,
			Namespace:   s1.Namespace,
			ServicePort: &s1.Spec.Ports[0],
		},
		LoadBalancerStrategy: "Cookie",
	}

	tests := map[string]struct {
		route *dag.Route
		want  *envoy_api_v2_route.Route_Route
	}{
		"single service": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
				},
			},
		},
		"websocket": {
			route: &dag.Route{
				Websocket: true,
				Clusters:  []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					UpgradeConfigs: []*envoy_api_v2_route.RouteAction_UpgradeConfig{{
						UpgradeType: "websocket",
					}},
				},
			},
		},
		"multiple": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
					},
					Weight: 90,
				}, {
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace, // it's valid to mention the same service several times per route.
						ServicePort: &s1.Spec.Ports[0],
					},
				}},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(0),
							}, {
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(90),
							}},
							TotalWeight: protobuf.UInt32(90),
						},
					},
				},
			},
		},
		"multiple websocket": {
			route: &dag.Route{
				Websocket: true,
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
					},
					Weight: 90,
				}, {
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace, // it's valid to mention the same service several times per route.
						ServicePort: &s1.Spec.Ports[0],
					},
				}},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(0),
							}, {
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(90),
							}},
							TotalWeight: protobuf.UInt32(90),
						},
					},
					UpgradeConfigs: []*envoy_api_v2_route.RouteAction_UpgradeConfig{{
						UpgradeType: "websocket",
					}},
				},
			},
		},
		"single service without retry-on": {
			route: &dag.Route{
				RetryPolicy: &dag.RetryPolicy{
					NumRetries:    7,                // ignored
					PerTryTimeout: 10 * time.Second, // ignored
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
				},
			},
		},
		"retry-on: 503": {
			route: &dag.Route{
				RetryPolicy: &dag.RetryPolicy{
					RetryOn:       "503",
					NumRetries:    6,
					PerTryTimeout: 100 * time.Millisecond,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					RetryPolicy: &envoy_api_v2_route.RetryPolicy{
						RetryOn:       "503",
						NumRetries:    protobuf.UInt32(6),
						PerTryTimeout: protobuf.Duration(100 * time.Millisecond),
					},
				},
			},
		},
		"timeout 90s": {
			route: &dag.Route{
				TimeoutPolicy: &dag.TimeoutPolicy{
					Timeout: 90 * time.Second,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					Timeout: protobuf.Duration(90 * time.Second),
				},
			},
		},
		"timeout infinity": {
			route: &dag.Route{
				TimeoutPolicy: &dag.TimeoutPolicy{
					Timeout: -1,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					Timeout: protobuf.Duration(0),
				},
			},
		},
		"single service w/ session affinity": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/e4f81994fe",
					},
					HashPolicy: []*envoy_api_v2_route.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
		"multiple service w/ session affinity": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2, c2},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}, {
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}},
							TotalWeight: protobuf.UInt32(2),
						},
					},
					HashPolicy: []*envoy_api_v2_route.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
		"mixed service w/ session affinity": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2, c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(1),
							}, {
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}},
							TotalWeight: protobuf.UInt32(2),
						},
					},
					HashPolicy: []*envoy_api_v2_route.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := RouteRoute(tc.route)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestWeightedClusters(t *testing.T) {
	tests := map[string]struct {
		clusters []*dag.Cluster
		want     *envoy_api_v2_route.WeightedCluster
	}{
		"multiple services w/o weights": {
			clusters: []*dag.Cluster{{
				Upstream: &dag.Service{
					Name:      "kuard",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			}, {
				Upstream: &dag.Service{
					Name:      "nginx",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			}},
			want: &envoy_api_v2_route.WeightedCluster{
				Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
					Name:   "default/kuard/8080/da39a3ee5e",
					Weight: protobuf.UInt32(1),
				}, {
					Name:   "default/nginx/8080/da39a3ee5e",
					Weight: protobuf.UInt32(1),
				}},
				TotalWeight: protobuf.UInt32(2),
			},
		},
		"multiple weighted services": {
			clusters: []*dag.Cluster{{
				Upstream: &dag.Service{
					Name:      "kuard",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 80,
			}, {
				Upstream: &dag.Service{
					Name:      "nginx",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 20,
			}},
			want: &envoy_api_v2_route.WeightedCluster{
				Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
					Name:   "default/kuard/8080/da39a3ee5e",
					Weight: protobuf.UInt32(80),
				}, {
					Name:   "default/nginx/8080/da39a3ee5e",
					Weight: protobuf.UInt32(20),
				}},
				TotalWeight: protobuf.UInt32(100),
			},
		},
		"multiple weighted services and one with no weight specified": {
			clusters: []*dag.Cluster{{
				Upstream: &dag.Service{
					Name:      "kuard",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 80,
			}, {
				Upstream: &dag.Service{
					Name:      "nginx",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 20,
			}, {
				Upstream: &dag.Service{
					Name:      "notraffic",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			}},
			want: &envoy_api_v2_route.WeightedCluster{
				Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
					Name:   "default/kuard/8080/da39a3ee5e",
					Weight: protobuf.UInt32(80),
				}, {
					Name:   "default/nginx/8080/da39a3ee5e",
					Weight: protobuf.UInt32(20),
				}, {
					Name:   "default/notraffic/8080/da39a3ee5e",
					Weight: protobuf.UInt32(0),
				}},
				TotalWeight: protobuf.UInt32(100),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := weightedClusters(tc.clusters)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestVirtualHost(t *testing.T) {
	tests := map[string]struct {
		hostname string
		port     int
		want     *envoy_api_v2_route.VirtualHost
	}{
		"default hostname": {
			hostname: "*",
			port:     9999,
			want: &envoy_api_v2_route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
			},
		},
		"www.example.com": {
			hostname: "www.example.com",
			port:     9999,
			want: &envoy_api_v2_route.VirtualHost{
				Name:    "www.example.com",
				Domains: []string{"www.example.com", "www.example.com:*"},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := VirtualHost(tc.hostname)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestUpgradeHTTPS(t *testing.T) {
	got := UpgradeHTTPS()
	want := &envoy_api_v2_route.Route_Redirect{
		Redirect: &envoy_api_v2_route.RedirectAction{
			SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestPerFilterConfig(t *testing.T) {
	msi := map[string]interface{}{
		"filter": map[string]interface{}{
			"map": map[string]interface{}{
				"float": 9.0,
				"bool":  true,
			},
			"list": []interface{}{
				"string",
			},
		},
	}
	got := make(map[string]*types.Struct)
	for k, v := range msi {
		s := new(types.Struct)
		got[k] = s
		recurseIface(s, v)
	}
	want := map[string]*types.Struct{
		"filter": {
			Fields: map[string]*types.Value{
				"map": {
					Kind: &types.Value_StructValue{
						&types.Struct{
							Fields: map[string]*types.Value{
								"float": {
									Kind: &types.Value_NumberValue{9.0},
								},
								"bool": {
									Kind: &types.Value_BoolValue{true},
								},
							},
						},
					},
				},
				"list": {
					Kind: &types.Value_ListValue{
						&types.ListValue{
							Values: []*types.Value{
								{
									Kind: &types.Value_StringValue{"string"},
								},
							},
						},
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}
