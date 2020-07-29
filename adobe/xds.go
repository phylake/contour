package adobe

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_types "github.com/envoyproxy/go-control-plane/envoy/type"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
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
// Add Jitter

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
// override RouteConfigName in HttpConnectionManager
// drop lua filter

// RDS customization
// no default timeout, comes from ingressroute
// no default HashPolicy, comes from ingressroute
// add route.VirtualHost.RetryPolicy, merge route.RouteAction.RetryPolicy if needed
// ClassAnnotation - test skipped
// root to root delegation - test skipped
// override RouteConfiguration.Name

// This is re-used in the e2e tests
var (
	RetryPolicy = &envoy_api_v2_route.RetryPolicy{
		RetryOn:                       "reset",
		NumRetries:                    protobuf.UInt32(3),
		HostSelectionRetryMaxAttempts: 3,
		RetryHostPredicate: []*envoy_api_v2_route.RetryPolicy_RetryHostPredicate{{
			Name: "envoy.retry_host_predicates.previous_hosts",
		}},
	}
)

// SDS customization
// none

// AdobefyXDS adds/drops Adobe customization to an xDS response object
// this is intended to modify the "want" response in all the tests
func AdobefyXDS(t *testing.T, resp *v2.DiscoveryResponse) {
	// First, un-marshall back to proto
	rec := unResources(t, resp.Resources...)

	// xDS specific customization
	switch resp.TypeUrl {
	case resource.ClusterType:
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

	case resource.ListenerType:
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

		// HttpConnectionManager:
		//  * override RouteConfigName
		//  * drop lua filter
		for _, l := range rec {
			listener := l.(*v2.Listener)
			for _, fc := range listener.GetFilterChains() {
				for _, f := range fc.GetFilters() {
					if f.GetName() == wellknown.HTTPConnectionManager {
						hcm := protobuf.MustUnmarshalAny(f.GetTypedConfig()).(*http.HttpConnectionManager)
						modified := false

						// override RouteConfigName
						rds := hcm.GetRds()
						if strings.HasPrefix(rds.GetRouteConfigName(), "https/") {
							rds.RouteConfigName = "ingress_https"
							hcm.RouteSpecifier = &http.HttpConnectionManager_Rds{
								Rds: rds,
							}
							modified = true
						}

						// drop lua filter
						luaFilterIndex := -1
						for i, hf := range hcm.GetHttpFilters() {
							if hf.GetName() == "envoy.filters.http.lua" {
								luaFilterIndex = i
							}
						}
						if luaFilterIndex != -1 {
							hcm.HttpFilters = append(hcm.HttpFilters[:luaFilterIndex], hcm.HttpFilters[luaFilterIndex+1:]...)
							modified = true
						}

						// rewrite config
						if modified {
							f.ConfigType = &envoy_api_v2_listener.Filter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(hcm),
							}
						}
					}
				}
			}
		}

	case resource.RouteType:
		for _, r := range rec {
			rc := r.(*v2.RouteConfiguration)
			for _, vh := range rc.VirtualHosts {
				vh.RetryPolicy = RetryPolicy
				for _, r := range vh.Routes {
					if rr, ok := r.GetAction().(*envoy_api_v2_route.Route_Route); ok {
						rr.Route.IdleTimeout = nil
						// merge RetryPolicy, but only if one already exists
						if rr.Route.RetryPolicy != nil {
							// RetryOn
							retryOnValues := []string{
								rr.Route.RetryPolicy.RetryOn,
								RetryPolicy.RetryOn,
							}
							rr.Route.RetryPolicy.RetryOn = strings.Join(retryOnValues, ",")
							// NumRetries
							if rr.Route.RetryPolicy.NumRetries == nil {
								rr.Route.RetryPolicy.NumRetries = RetryPolicy.NumRetries
							}
							// HostSelectionRetryMaxAttempts
							rr.Route.RetryPolicy.HostSelectionRetryMaxAttempts = RetryPolicy.HostSelectionRetryMaxAttempts
						}
						rr.Route.HashPolicy = nil
					}
				}
			}
			// override Name
			if strings.HasPrefix(rc.Name, "https/") {
				rc.Name = "ingress_https"
			}
		}
		// resort the list
		rcs := make([](*v2.RouteConfiguration), len(rec))
		for i := range rec {
			rcs[i] = rec[i].(*v2.RouteConfiguration)
		}
		sort.Stable(routeConfigurationSorter(rcs))
		rec = protobuf.AsMessages(rcs)
	}

	// Re-compute version info
	resp.VersionInfo = Hash(rec)

	// Now, re-marshall
	resp.Resources = resources(t, rec...)
}

// Hash is the same one as internal/grpc/xds_adobe.go
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
		protos = append(protos, protobuf.MustUnmarshalAny(a))
	}
	return protos
}

// Stolen from internal/e2e/e2e.go
func resources(t *testing.T, protos ...proto.Message) []*any.Any {
	t.Helper()
	var anys []*any.Any
	for _, a := range protos {
		anys = append(anys, protobuf.MustMarshalAny(a))
	}
	return anys
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Stolen from internal/sorter/sort.go
// Sorts the given route configuration values by name.
type routeConfigurationSorter []*v2.RouteConfiguration

func (s routeConfigurationSorter) Len() int           { return len(s) }
func (s routeConfigurationSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s routeConfigurationSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }
