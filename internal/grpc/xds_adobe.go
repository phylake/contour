package grpc

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/gogo/protobuf/proto"
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

// default date when none is used in the version
var oldDate time.Time = time.Date(2000, time.January, 1, 1, 0, 0, 0, time.UTC)

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

func hash(data []proto.Message) string {
	jsonBytes, _ := json.Marshal(data)
	hash := md5.Sum(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// splitVersionInfo splits a xDS VersionInfo into a hash and timestamp.
// The timestamp is the date when the xDS response was sent: if it cannot be parsed
// or doesn't exist, it is set to an old date (1/1/2000)
func splitVersionInfo(vinfo string) (string, time.Time) {
	v := strings.Split(vinfo, ",")
	switch len(v) {
	case 0:
		// First Envoy request
		return "", oldDate
	case 1:
		// Just a version
		return v[0], oldDate
	default:
		var t time.Time
		i, err := strconv.ParseInt(v[1], 10, 64)
		if err != nil {
			t = oldDate
		}
		t = time.Unix(i, 0)
		return v[0], t
	}
}

// joinVersionInfo performs the opposite operation of splitVersionInfo.
func joinVersionInfo(hash string, ts time.Time) string {
	return strings.Join([]string{hash, strconv.FormatInt(ts.Unix(), 10)}, ",")
}
