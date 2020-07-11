package protobuf

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
)

// MustUnmarshalAny is the reverse of protobuf.MustMarshalAny()
func MustUnmarshalAny(a *any.Any) proto.Message {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	if err != nil {
		panic(err.Error())
	}
	return x.Message
}
