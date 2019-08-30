package grpc

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
)

var oldDate time.Time = time.Date(2000, time.January, 1, 1, 0, 0, 0, time.UTC)

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
