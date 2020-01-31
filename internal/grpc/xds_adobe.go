package grpc

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// streamId uniquely identifies a stream
type streamId struct {
	TypeUrl string
	NodeId  string
}

// A cache of data already sent, used for sending updates in an orderly manner
// https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#eventual-consistency-considerations
var streamCache = make(map[streamId]map[string]string)
var mutex = &sync.Mutex{}

// sleep time and max_wait time when synchonization is required
const (
	waitSleepTime = 100 * time.Millisecond
	maxWaitTime   = 2 * time.Minute
	// time is stored as nanoseconds: any adjustments to the above consts should
	// ensure maxWaitCount fits into 16 bits (e.g. < 32767)
	maxWaitCount = int16(maxWaitTime / waitSleepTime)

	// log throttling config
	logInterval = 1 * time.Second
	// divisor is used in the modulo operation
	logDivisor = int16(logInterval / waitSleepTime)
)

// "In order for EDS resources to be known or tracked by Envoy, there must exist an applied Cluster definition (e.g. sourced via CDS).
// A similar relationship exists between RDS and Listeners (e.g. sourced via LDS).""
// EDS will wait for CDS
// RDS will wait for LDS
// return true if synchronization was necessary
func synchronizeXDS(req *envoy_api_v2.DiscoveryRequest, res Resource, log *logrus.Entry) (needSync bool) {
	needSync = false
	if _, ok := os.LookupEnv("ZZZ_NO_SYNC_XDS"); ok {
		return
	}
	var waitCount int16 = -1
	freeToGo := false

	for !freeToGo {
		waitCount++
		mutex.Lock()
		switch req.TypeUrl {
		case envoy.ClusterType:
			freeToGo = true

		case envoy.EndpointType:
			// After CDS
			if isKnown(envoy.ClusterType, req.Node.Id, req.ResourceNames) {
				freeToGo = true
				break
			}
			throttledLog(waitCount, log, "wait_on_cds")

		case envoy.ListenerType:
			// After CDS and EDS
			// FOR NOW: only ensure some CDS and EDS were sent (can't tie a listener back to a cluster or endpoint)
			if len(streamCache[streamId{TypeUrl: envoy.ClusterType, NodeId: req.Node.Id}]) > 0 &&
				len(streamCache[streamId{TypeUrl: envoy.EndpointType, NodeId: req.Node.Id}]) > 0 {
				freeToGo = true
				break
			}
			// Split these logs to ease potential troubleshooting
			if len(streamCache[streamId{TypeUrl: envoy.ClusterType, NodeId: req.Node.Id}]) > 0 {
				throttledLog(waitCount, log, "wait_on_cds")
			}
			if len(streamCache[streamId{TypeUrl: envoy.EndpointType, NodeId: req.Node.Id}]) > 0 {
				throttledLog(waitCount, log, "wait_on_eds")
			}

		case envoy.RouteType:
			// After LDS and CDS
			if isKnown(envoy.ListenerType, req.Node.Id, req.ResourceNames) {
				// build a list of all the clusters referenced in this route config
				resources := getResources(res, req.ResourceNames)
				clusterSet := make(map[string]struct{})
				for _, rec := range resources {
					rc := rec.(*envoy_api_v2.RouteConfiguration)
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
					if isKnown(envoy.ClusterType, req.Node.Id, clusters) {
						freeToGo = true
						break
					}
					unknown := diff(envoy.ClusterType, req.Node.Id, clusters)
					throttledLogWithFields(waitCount, log, logrus.Fields{"unknown": unknown}, "wait_on_cds")
					break

					// TODO: also check route.GetVhds()
				}
			}
			throttledLog(waitCount, log, "wait_on_lds")

		case envoy.SecretType:
			// uh? let's do after listener
			if len(streamCache[streamId{TypeUrl: envoy.ListenerType, NodeId: req.Node.Id}]) >= 2 {
				freeToGo = true
				break
			}
			log.WithField("wait_count", waitCount).WithField("wait_count_max", maxWaitCount).Info("wait_on_lds")

		default:
			log.Warn("xDS response ordering: type not handled")
			// might as well just send for now
			freeToGo = true
		}
		mutex.Unlock()

		if waitCount >= 1 {
			needSync = true
			time.Sleep(waitSleepTime)
			// fail-safe: don't wait indefinitely
			if waitCount > maxWaitCount {
				log.WithField("wait_count", waitCount).WithField("wait_count_max", maxWaitCount).Warn("max_wait_time_exceeded")
				freeToGo = true
			}
		}
	}
	return
}

// unsafe: assumes locking already in progress
func isKnown(typeURL string, nodeId string, names []string) bool {
	stId := streamId{
		TypeUrl: typeURL,
		NodeId:  nodeId,
	}

	switch typeURL {
	case envoy.ClusterType:
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
	case envoy.ListenerType:
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
	case envoy.ClusterType:
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
		stId.TypeUrl == envoy.ClusterType ||
		stId.TypeUrl == envoy.ListenerType {
		streamCache[stId] = make(map[string]string)
	}

	for _, r := range data {
		switch v := r.(type) {
		case *envoy_api_v2.Cluster:
			// cluster-name: some unique name across the entire cluster
			// https://github.com/envoyproxy/go-control-plane/blob/c7e2a120463a2209c6a0871d778f4eab96457e6b/envoy/api/v2/cds.pb.go#L323-L328
			// service-name: the actual cluster name presented to EDS
			// https://github.com/envoyproxy/go-control-plane/blob/c7e2a120463a2209c6a0871d778f4eab96457e6b/envoy/api/v2/cds.pb.go#L1047
			//
			// map[cluster-name] => service-name
			streamCache[stId][v.Name] = v.GetEdsClusterConfig().GetServiceName()

		case *envoy_api_v2.ClusterLoadAssignment:
			nb := len(v.GetEndpoints())
			current, _ := strconv.Atoi(streamCache[stId]["total"])
			streamCache[stId]["total"] = strconv.Itoa(current + nb)

		case *envoy_api_v2.Listener:
			streamCache[stId][v.Name] = "known"

		case *envoy_api_v2.RouteConfiguration:
			// not caching routes for now

		case *envoy_api_v2_auth.Secret:
			// not caching secrets for now

		default:
			// fmt.Println("no idea what to cache", v)
		}
	}
}

func lessProtoMessage(x, y proto.Message) bool {
	switch xm := x.(type) {
	case *envoy_api_v2.Cluster:
		ym := y.(*envoy_api_v2.Cluster)
		return xm.Name < ym.Name
	case *envoy_api_v2.Listener:
		ym := y.(*envoy_api_v2.Listener)
		return xm.Name < ym.Name
	default:
		return true
	}
}

func hash(data []proto.Message) string {
	jsonBytes, _ := json.Marshal(data)
	hash := md5.Sum(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// Fetches the resources for the given ResourceNames
// dupe of https://github.com/projectcontour/contour/blob/v1.1.0/internal/grpc/xds.go#L122-L130
func getResources(r Resource, names []string) (resources []proto.Message) {
	switch len(names) {
	case 0:
		// no resource hints supplied, return the full
		// contents of the resource
		resources = r.Contents()
	default:
		// resource hints supplied, return exactly those
		resources = r.Query(names)
	}
	return resources
}

func throttledLog(wc int16, log *logrus.Entry, msg string) {
	throttledLogWithFields(wc, log, logrus.Fields{}, msg)
}

func throttledLogWithFields(wc int16, log *logrus.Entry, fields logrus.Fields, msg string) {
	if wc%logDivisor == 0 {
		log.WithField("wait_count", wc).WithField("wait_count_max", maxWaitCount).WithFields(fields).Info(msg)
	}
}
