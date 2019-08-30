// Copyright © 2018 Heptio
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

package grpc

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
)

// Resource represents a source of proto.Messages that can be registered
// for interest.
type Resource interface {
	// Contents returns the contents of this resource.
	Contents() []proto.Message

	// Query returns an entry for each resource name supplied.
	Query(names []string) []proto.Message

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)

	// TypeURL returns the typeURL of messages returned from Values.
	TypeURL() string
}

// xdsHandler implements the Envoy xDS gRPC protocol.
type xdsHandler struct {
	logrus.FieldLogger
	connections counter
	resources   map[string]Resource // registered resource types
}

type grpcStream interface {
	Context() context.Context
	Send(*v2.DiscoveryResponse) error
	Recv() (*v2.DiscoveryRequest, error)
}

// streamId uniquely identifies a stream
type streamId struct {
	TypeUrl string
	NodeId  string
}

// A cache of data already sent, used for sending updates in an orderly manner
// https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#eventual-consistency-considerations
var streamCache = make(map[streamId]map[string]string)
var mutex = &sync.Mutex{}

// stream processes a stream of DiscoveryRequests.
func (xh *xdsHandler) stream(st grpcStream) (err error) {
	// bump connection counter and set it as a field on the logger
	log := xh.WithField("connection", xh.connections.next())

	// set up some nice function exit handling which notifies if the
	// stream terminated on error or not.
	defer func() {
		if err != nil {
			log.WithError(err).Error("stream terminated")
		} else {
			log.Info("stream terminated")
		}
	}()

	ch := make(chan int, 1)

	// internally all registration values start at zero so sending
	// a last that is less than zero will guarantee that each stream
	// will generate a response immediately, then wait.
	last := -1
	ctx := st.Context()

	// A cache of resources last sent on this specific stream
	// Note that this will reset if the stream is terminated (a good thing)
	var previous_resources []proto.Message

	// now stick in this loop until the client disconnects.
	for {
		// first we wait for the request from Envoy, this is part of
		// the xDS protocol.
		req, err := st.Recv()
		if err != nil {
			return err
		}

		// note: redeclare log in this scope so the next time around the loop all is forgotten.
		log := log.WithField("version_info", req.VersionInfo).WithField("response_nonce", req.ResponseNonce)
		if req.Node != nil {
			log = log.WithField("node_id", req.Node.Id)
		}

		if err := req.ErrorDetail; err != nil {
			// if Envoy rejected the last update log the details here.
			// TODO(dfc) issue 1176: handle xDS ACK/NACK
			log.WithField("code", err.Code).Error(err.Message)
		}

		// from the request we derive the resource to stream which have
		// been registered according to the typeURL.
		r, ok := xh.resources[req.TypeUrl]
		if !ok {
			return fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
		}
		log = log.WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl)

		stId := streamId{
			TypeUrl: req.TypeUrl,
			NodeId:  req.Node.Id,
		}

		// timer to prevent stacking LDS updates
		var ldsTimer *time.Timer

	WaitForChange:
		log.Info("stream_wait")

		// now we wait for a notification, if this is the first request received on this
		// connection last will be less than zero and that will trigger a response immediately.
		r.Register(ch, last)
		select {
		case last = <-ch:
			// boom, something in the cache has changed.
			// TODO(dfc) the thing that has changed may not be in the scope of the filter
			// so we're going to be sending an update that is a no-op. See #426

			var resources []proto.Message
			switch len(req.ResourceNames) {
			case 0:
				// no resource hints supplied, return the full
				// contents of the resource
				resources = r.Contents()
			default:
				// resource hints supplied, return exactly those
				resources = r.Query(req.ResourceNames)
			}

			// parse out the request VersionInfo {hash},{timestamp}
			now := time.Now()
			reqHash, reqTS := splitVersionInfo(req.VersionInfo)
			resHash, resTS := hash(resources), now

			// Skip this response entirely if we already sent the exact same data previously
			if resHash == reqHash {
				log.WithField("count", len(resources)).Info("skip")
				goto WaitForChange
			}

			// Keeping this for now to ensure the version-info hashing can take over
			// Will be removed eventually. TODO(lrouquet)
			if cmp.Equal(previous_resources, resources, cmpopts.SortSlices(lessProtoMessage)) {
				log.WithField("count", len(resources)).Info("skip_legacy")
				goto WaitForChange
			}

			// Check for stacking LDS update
			// This is a long wait time, so ensure we're sending the latest LDS config
			// Strategy:
			// * skip the response (so we can receive further cache updates)
			// * manually trigger the update after the wait time
			if req.TypeUrl == "type.googleapis.com/envoy.api.v2.Listener" {
				// Wait at least 12min since the last update
				earliestNextUpdate := reqTS.Add(12 * time.Minute)
				// Since AfterFunc() is not perfectly accurate, we add 1s to now (we already waited 12min, so 1s is insignificant)
				// This is to ensure we do send the update when the timer triggered
				if earliestNextUpdate.After(now.Add(1 * time.Second)) {
					fmt.Println("*** LDS NEED TO WAIT ***", last, earliestNextUpdate, now)
					// Ok, we need to wait - check if the timer already started
					if ldsTimer == nil {
						fmt.Println("*** LDS -- CREATING TIMER ***")
						waitDuration := time.Until(earliestNextUpdate)
						manualTrigger := func() {
							// Wait an extra 1s before triggering: AfterFunc() is not really accurate
							fmt.Println("*** TIMER TRIGGERED ***", last)
							ch <- last // TODO: figure out what to send here
						}
						ldsTimer = time.AfterFunc(waitDuration, manualTrigger)
					}
					// now wait
					goto WaitForChange
				}
				// Either the last response was sent long ago, or the timer triggered: reset it anyhow
				fmt.Println("*** LDS: NO WAITING OR TIMER EXPIRED ***")
				ldsTimer = nil
			}

			any, err := toAny(r.TypeURL(), resources)
			if err != nil {
				return err
			}

			resp := &v2.DiscoveryResponse{
				VersionInfo: joinVersionInfo(resHash, resTS),
				Resources:   any,
				TypeUrl:     r.TypeURL(),
				Nonce:       strconv.Itoa(last),
			}

			// Ensure we send this update in the right order
			// "In order for EDS resources to be known or tracked by Envoy, there must exist an applied Cluster definition (e.g. sourced via CDS).
			// A similar relationship exists between RDS and Listeners (e.g. sourced via LDS).""
			// EDS will wait for CDS
			// RDS will wait for LDS

			freeToGo := false
			for !freeToGo {
				mutex.Lock()
				switch req.TypeUrl {
				case "type.googleapis.com/envoy.api.v2.Cluster":
					freeToGo = true

				case "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment":
					// After CDS
					if isKnown("type.googleapis.com/envoy.api.v2.Cluster", req.Node.Id, req.ResourceNames) {
						freeToGo = true
					}

				case "type.googleapis.com/envoy.api.v2.Listener":
					// After CDS and EDS
					// FOR NOW: only ensure some CDS and EDS were sent (can't tie a listener back to a cluster or endpoint)
					if len(streamCache[streamId{TypeUrl: "type.googleapis.com/envoy.api.v2.Cluster", NodeId: req.Node.Id}]) > 0 &&
						len(streamCache[streamId{TypeUrl: "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment", NodeId: req.Node.Id}]) > 0 {
						freeToGo = true
					}

				case "type.googleapis.com/envoy.api.v2.RouteConfiguration":
					// After LDS and CDS
					if isKnown("type.googleapis.com/envoy.api.v2.Listener", req.Node.Id, req.ResourceNames) {
						// build a list of all the clusters referenced in this route config
						clusterSet := make(map[string]struct{})
						for _, rec := range resources {
							rc := rec.(*v2.RouteConfiguration)
							for _, v := range rc.GetVirtualHosts() {
								for _, r := range v.GetRoutes() {
									cl := r.GetRoute().GetCluster()
									if cl != "" {
										clusterSet[cl] = struct{}{}
									}
								}
							}

							// Convert the set of clusters to a slice
							clusters := make([]string, 0, len(clusterSet))
							for k := range clusterSet {
								clusters = append(clusters, k)
							}

							// Ensure all these clusters were already sent
							if isKnown("type.googleapis.com/envoy.api.v2.Cluster", req.Node.Id, clusters) {
								freeToGo = true
							} else {
								unknown := diff("type.googleapis.com/envoy.api.v2.Cluster", req.Node.Id, clusters)
								log.WithField("unknown", unknown).Info("wait_on_cds")
							}
							// TODO: also check route.GetVhds()
						}
					}

				case "type.googleapis.com/envoy.api.v2.auth.Secret":
					// uh? let's do after listener
					if len(streamCache[streamId{TypeUrl: "type.googleapis.com/envoy.api.v2.Listener", NodeId: req.Node.Id}]) >= 2 {
						freeToGo = true
					}

				default:
					log.Warn("xDS response ordering: type not handled")
					// might as well just send for now
					freeToGo = true
				}
				mutex.Unlock()
			}

			if err := st.Send(resp); err != nil {
				return err
			}
			log.WithField("count", len(resources)).WithField("version_info_resp", resp.VersionInfo).Info("response")

			// cache what was sent
			cacheData(stId, resources)
			previous_resources = resources

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// unsafe: assumes locking already in progress
func isKnown(typeURL string, nodeId string, names []string) bool {
	stId := streamId{
		TypeUrl: typeURL,
		NodeId:  nodeId,
	}

	switch typeURL {
	case "type.googleapis.com/envoy.api.v2.Cluster":
		// either the key (unique name) or the value (name known to EDS) will work
		ret := true
		for _, n := range names {
			foundMatch := false
			for k, v := range streamCache[stId] {
				if n == k || n == v {
					foundMatch = true
					break
				}
			}
			ret = ret && foundMatch
			// no need to check other names if that one wasn't found
			if !ret {
				return ret
			}
		}
		return ret
	case "type.googleapis.com/envoy.api.v2.Listener":
		ret := true
		for _, n := range names {
			_, ok := streamCache[stId][n]
			ret = ret && ok
		}
		return ret
	default:
		fmt.Println("isKnown: unknown typeURL", typeURL)
	}
	return true
}

func diff(typeURL string, nodeId string, names []string) []string {
	stId := streamId{
		TypeUrl: typeURL,
		NodeId:  nodeId,
	}

	switch typeURL {
	case "type.googleapis.com/envoy.api.v2.Cluster":
		// either the key (unique name) or the value (name known to EDS) will work
		ret := make([]string, 0)
		for _, n := range names {
			foundMatch := false
			for k, v := range streamCache[stId] {
				if n == k || n == v {
					foundMatch = true
					break
				}
			}
			if !foundMatch {
				ret = append(ret, n)
			}
		}
		return ret
	default:
		fmt.Println("diff: not implemented", typeURL)
	}
	return []string{}
}

func cacheData(stId streamId, data []proto.Message) {
	mutex.Lock()
	defer mutex.Unlock()

	// clear out CDS and LDS
	if streamCache[stId] == nil ||
		stId.TypeUrl == "type.googleapis.com/envoy.api.v2.Cluster" ||
		stId.TypeUrl == "type.googleapis.com/envoy.api.v2.Listener" {
		streamCache[stId] = make(map[string]string)
	}

	for _, r := range data {
		switch v := r.(type) {
		case *v2.Cluster:
			// cluster-name: some unique name across the entire cluster
			// https://github.com/envoyproxy/go-control-plane/blob/c7e2a120463a2209c6a0871d778f4eab96457e6b/envoy/api/v2/cds.pb.go#L323-L328
			// service-name: the actual cluster name presented to EDS
			// https://github.com/envoyproxy/go-control-plane/blob/c7e2a120463a2209c6a0871d778f4eab96457e6b/envoy/api/v2/cds.pb.go#L1047
			//
			// map[cluster-name] => service-name
			streamCache[stId][v.Name] = v.GetEdsClusterConfig().GetServiceName()

		case *v2.ClusterLoadAssignment:
			nb := len(v.GetEndpoints())
			current, _ := strconv.Atoi(streamCache[stId]["total"])
			streamCache[stId]["total"] = strconv.Itoa(current + nb)

		case *v2.Listener:
			streamCache[stId][v.Name] = "known"

		case *v2.RouteConfiguration:
			// not caching routes for now

		case *auth.Secret:
			// not caching secrets for now

		default:
			// fmt.Println("no idea what to cache", v)
		}
	}
}

func lessProtoMessage(x, y proto.Message) bool {
	switch xm := x.(type) {
	case *v2.Cluster:
		ym := y.(*v2.Cluster)
		return xm.Name < ym.Name
	case *v2.Listener:
		ym := y.(*v2.Listener)
		return xm.Name < ym.Name
	default:
		return true
	}
}

// toAny converts the contents of a resourcer's Values to the
// respective slice of types.Any.
func toAny(typeURL string, values []proto.Message) ([]types.Any, error) {
	var resources []types.Any
	for _, value := range values {
		v, err := proto.Marshal(value)
		if err != nil {
			return nil, err
		}
		resources = append(resources, types.Any{TypeUrl: typeURL, Value: v})
	}
	return resources, nil
}

// counter holds an atomically incrementing counter.
type counter uint64

func (c *counter) next() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}
