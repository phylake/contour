package e2e

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_types "github.com/envoyproxy/go-control-plane/envoy/type"
	envoy "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/protobuf"
)

// CDS customization
// add CircuitBreakers
// set DrainConnectionsOnHostRemoval
// cluster.IdleTimeout (TODO test)
// add ExpectedStatuses
var circuitBreakers = &envoy_cluster.CircuitBreakers{
	Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
		MaxConnections: protobuf.UInt32(1000000),
		MaxRequests:    protobuf.UInt32(1000000),
	}},
}

var expectedStatuses = []*envoy_types.Int64Range{
	{
		Start: 200,
		End:   400,
	},
}

// EDS customization
// none

// LDS customization
// drop StatsListener
// add envoy.listener.ip_allow_deny filter if CIDR_LIST_PATH is set (TODO test)

// RDS customization
// no default timeout, comes from ingressroute (removal done in rds_test.go/clustertimeout())
// no default HashPolicy, comes from ingressroute (removal done in rds_test.go/withSessionAffinity())
// no RetryPolicy (removal done in rds_test.go/routeretry())
// ClassAnnotation - test skipped
// root to root delegation - test skipped

// SDS customization
// none

// adobefyXDS adds/drops Adobe customization to an xDS response object
// this is intended to modify the "want" response in all the tests
func adobefyXDS(t *testing.T, resp *v2.DiscoveryResponse) {
	// First, un-marshall back to proto
	rec := unResources(t, resp.Resources)

	// xDS specific customization
	switch resp.TypeUrl {
	case envoy.ClusterType:
		for _, c := range rec {
			cluster := c.(*v2.Cluster)
			cluster.CircuitBreakers = circuitBreakers
			cluster.DrainConnectionsOnHostRemoval = true
			if cluster.HealthChecks != nil {
				for _, h := range cluster.HealthChecks {
					h.GetHttpHealthCheck().ExpectedStatuses = expectedStatuses
				}
			}
		}
	case envoy.ListenerType:
		// Find and remove the stats-listener
		statsListenerIndex := -1
		for i, l := range rec {
			listener := l.(*v2.Listener)
			if listener.Name == staticListener().Name {
				statsListenerIndex = i
			}
		}
		if statsListenerIndex != -1 {
			// drop, but keep the order - no the best performing, but idiomatic
			rec = append(rec[:statsListenerIndex], rec[statsListenerIndex+1:]...)
		}
	}

	// Re-compute version info
	resp.VersionInfo = hash(rec)

	// Now, re-marshall
	resp.Resources = resources(t, rec...)
}

// hash is the same one as internal/grpc/xds_adobe.go
// except that an empty input is treated as nil
// that's because resources() will return an empty array no matter what
func hash(data []proto.Message) string {
	if len(data) == 0 {
		data = nil
	}
	jsonBytes, _ := json.Marshal(data)
	hash := md5.Sum(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// unResources performs the opposite operation of resources().
func unResources(t *testing.T, anys []*any.Any) []proto.Message {
	t.Helper()
	protos := make([]proto.Message, 0, len(anys))
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
