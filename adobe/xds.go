package adobe

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_types "github.com/envoyproxy/go-control-plane/envoy/type"
	envoy "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/protobuf"
)

const (
	staticListenerName = "stats-health"
)

// CDS customization
// add CircuitBreakers
// set DrainConnectionsOnHostRemoval
// add IdleTimeout via CommonHttpProtocolOptions
// cluster.IdleTimeout (TODO test)
// add ExpectedStatuses

// These are re-used in the e2e tests
var (
	CircuitBreakers = &envoy_cluster.CircuitBreakers{
		Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
			MaxConnections: protobuf.UInt32(1000000),
			MaxRequests:    protobuf.UInt32(1000000),
		}},
	}

	CommonHttpProtocolOptions = &envoy_api_v2_core.HttpProtocolOptions{
		IdleTimeout: protobuf.Duration(58 * time.Second),
	}

	ExpectedStatuses = []*envoy_types.Int64Range{
		{
			Start: 200,
			End:   400,
		},
	}
	InitialJitter                = protobuf.Duration(1 * time.Second)
	IntervalJitterPercent uint32 = 100
)

// EDS customization
// none

// LDS customization
// drop StatsListener
// add envoy.listener.ip_allow_deny filter if CIDR_LIST_PATH is set (TODO test)

// RDS customization
// no default timeout, comes from ingressroute
// no default HashPolicy, comes from ingressroute
// no RetryPolicy
// ClassAnnotation - test skipped
// root to root delegation - test skipped

// SDS customization
// none

// AdobefyXDS adds/drops Adobe customization to an xDS response object
// this is intended to modify the "want" response in all the tests
func AdobefyXDS(t *testing.T, resp *v2.DiscoveryResponse) {
	// First, un-marshall back to proto
	rec := unResources(t, resp.Resources...)

	// xDS specific customization
	switch resp.TypeUrl {
	case envoy.ClusterType:
		for _, c := range rec {
			cluster := c.(*v2.Cluster)
			cluster.CircuitBreakers = CircuitBreakers
			cluster.DrainConnectionsOnHostRemoval = true
			cluster.CommonHttpProtocolOptions = CommonHttpProtocolOptions
			if cluster.HealthChecks != nil {
				for _, h := range cluster.HealthChecks {
					h.InitialJitter = InitialJitter
					h.IntervalJitterPercent = IntervalJitterPercent
					h.GetHttpHealthCheck().ExpectedStatuses = ExpectedStatuses
				}
			}
		}
	case envoy.ListenerType:
		// Find and remove the stats-listener
		statsListenerIndex := -1
		for i, l := range rec {
			listener := l.(*v2.Listener)
			if listener.Name == staticListenerName {
				statsListenerIndex = i
			}
		}
		if statsListenerIndex != -1 {
			// drop, but keep the order - no the best performing, but idiomatic
			rec = append(rec[:statsListenerIndex], rec[statsListenerIndex+1:]...)
		}
	case envoy.RouteType:
		for _, r := range rec {
			rc := r.(*v2.RouteConfiguration)
			for _, vh := range rc.VirtualHosts {
				for _, r := range vh.Routes {
					if rr, ok := r.GetAction().(*envoy_api_v2_route.Route_Route); ok {
						rr.Route.IdleTimeout = nil
						rr.Route.RetryPolicy = nil
						rr.Route.HashPolicy = nil
					}
				}
			}
		}
	}

	// Re-compute version info
	resp.VersionInfo = Hash(rec)

	// Now, re-marshall
	resp.Resources = resources(t, rec...)
}

// hash is the same one as internal/grpc/xds_adobe.go
func Hash(data []proto.Message) string {
	jsonBytes, _ := json.Marshal(data)
	hash := md5.Sum(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// unResources performs the opposite operation of resources().
func unResources(t *testing.T, anys ...*any.Any) []proto.Message {
	t.Helper()
	var protos []proto.Message
	for _, a := range anys {
		protos = append(protos, toMessage(t, a))
	}
	return protos
}

// toMessage performs the opposite operation of toAny().
func toMessage(t *testing.T, a *any.Any) proto.Message {
	t.Helper()
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	check(t, err)
	return x.Message
}

//
// Stolen from internal/e2e/e2e.go
//
func resources(t *testing.T, protos ...proto.Message) []*any.Any {
	t.Helper()
	var anys []*any.Any
	for _, a := range protos {
		anys = append(anys, toAny(t, a))
	}
	return anys
}

func toAny(t *testing.T, pb proto.Message) *any.Any {
	t.Helper()
	a, err := ptypes.MarshalAny(pb)
	check(t, err)
	return a
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
