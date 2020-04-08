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
	"time"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_types "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

const (
	// Default healthcheck / lb algorithm values
	hcTimeout               = 2 * time.Second
	hcInterval              = 60 * time.Second
	hcInitialJitter         = 1 * time.Second // random start time between 0 and 1000 ms
	hcIntervalJitter        = 0               // random jitter between healthchecks in milliseconds is set to 0, since we set interval jitter in percent, by default
	hcIntervalJitterPercent = 100             // have the jitter be randomized across the entire interval
	hcUnhealthyThreshold    = 3
	hcHealthyThreshold      = 2
	hcHost                  = "contour-envoy-healthcheck"
)

// healthCheck returns a *envoy_api_v2_core.HealthCheck value.
func healthCheck(cluster *dag.Cluster) *envoy_api_v2_core.HealthCheck {
	hc := cluster.HealthCheckPolicy
	host := hcHost
	if hc.Host != "" {
		host = hc.Host
	}

	// TODO(dfc) why do we need to specify our own default, what is the default
	// that envoy applies if these fields are left nil?
	return &envoy_api_v2_core.HealthCheck{
		Timeout:               durationOrDefault(hc.Timeout, hcTimeout),
		Interval:              durationOrDefault(hc.Interval, hcInterval),
		InitialJitter:         durationOrDefault(hc.InitialJitterMilliseconds, hcInitialJitter),
		IntervalJitter:        durationOrDefault(hc.IntervalJitterMilliseconds, hcIntervalJitter),
		IntervalJitterPercent: percentOrDefault(hc.IntervalJitterPercent, hcIntervalJitterPercent),
		UnhealthyThreshold:    countOrDefault(hc.UnhealthyThreshold, hcUnhealthyThreshold),
		HealthyThreshold:      countOrDefault(hc.HealthyThreshold, hcHealthyThreshold),
		HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
				Path: hc.Path,
				Host: host,
				// [200, 400) so 200-399 to match K8s probes
				ExpectedStatuses: []*envoy_types.Int64Range{
					{
						Start: 200,
						End:   400,
					},
				},
			},
		},
	}
}

func durationOrDefault(d, def time.Duration) *duration.Duration {
	if d != 0 {
		return protobuf.Duration(d)
	}
	return protobuf.Duration(def)
}

func countOrDefault(count uint32, def uint32) *wrappers.UInt32Value {
	switch count {
	case 0:
		return protobuf.UInt32(def)
	default:
		return protobuf.UInt32(count)
	}
}

func percentOrDefault(percent uint32, def uint32) uint32 {
	switch percent {
	case 0:
		return def
	case 100.:
		return def
	default:
		return percent
	}
}
