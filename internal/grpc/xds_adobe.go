package grpc

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"

	"github.com/golang/protobuf/proto"
)

func hash(data []proto.Message) string {
	jsonBytes, _ := json.Marshal(data)
	hash := md5.Sum(jsonBytes)
	return hex.EncodeToString(hash[:])
}
