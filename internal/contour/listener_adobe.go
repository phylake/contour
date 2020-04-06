package contour

import (
	"encoding/json"
	"os"

	udpa_type_v1 "github.com/cncf/udpa/go/udpa/type/v1"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	_struct "github.com/golang/protobuf/ptypes/struct"
)

type (
	Cidr struct {
		AddressPrefix string  `json:"address_prefix"`
		PrefixLen     float64 `json:"prefix_len"`
	}

	IpAllowDenyConfig struct {
		AllowCidrs *[]Cidr `json:"allow_cidrs"`
		DenyCidrs  *[]Cidr `json:"deny_cidrs"`
	}
)

var ipAllowDenyListenerFilter *envoy_api_v2_listener.ListenerFilter

func init() {
	path := os.Getenv("CIDR_LIST_PATH")
	if path == "" {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		panic("CIDR_LIST_PATH was provided but os.Open failed " + err.Error())
	}
	defer f.Close()

	config := IpAllowDenyConfig{}
	err = json.NewDecoder(f).Decode(&config)
	if err != nil {
		panic("could not deserialize cidrs in CIDR_LIST_PATH " + path)
	}

	structFields := make(map[string]*_struct.Value)

	if config.AllowCidrs != nil {
		cidrToProto(*config.AllowCidrs, "allow_cidrs", structFields)
	}

	if config.DenyCidrs != nil {
		cidrToProto(*config.DenyCidrs, "deny_cidrs", structFields)
	}

	if len(structFields) > 0 {
		ipAllowDenyListenerFilter = new(envoy_api_v2_listener.ListenerFilter)
		ipAllowDenyListenerFilter.Name = "envoy.listener.ip_allow_deny"
		ipAllowDenyListenerFilter.ConfigType = &envoy_api_v2_listener.ListenerFilter_TypedConfig{
			TypedConfig: toAny(&udpa_type_v1.TypedStruct{
				TypeUrl: "envoy.config.filter.network.ip_allow_deny.v2.IpAllowDeny",
				Value: &_struct.Struct{
					Fields: structFields,
				},
			}),
		}
	}
}

func cidrToProto(cidrs []Cidr, key string, structFields map[string]*_struct.Value) {
	cidrList := &_struct.ListValue{
		Values: make([]*_struct.Value, 0),
	}
	structFields[key] = &_struct.Value{
		Kind: &_struct.Value_ListValue{
			ListValue: cidrList,
		},
	}

	for _, cidr := range cidrs {
		cidrStruct := &_struct.Struct{
			Fields: make(map[string]*_struct.Value),
		}
		cidrStruct.Fields["address_prefix"] = &_struct.Value{
			Kind: &_struct.Value_StringValue{
				StringValue: cidr.AddressPrefix,
			},
		}
		cidrStruct.Fields["prefix_len"] = &_struct.Value{
			Kind: &_struct.Value_NumberValue{
				NumberValue: cidr.PrefixLen,
			},
		}
		cidrList.Values = append(cidrList.Values, &_struct.Value{
			Kind: &_struct.Value_StructValue{
				StructValue: cidrStruct,
			},
		})
	}
}

func toAny(pb proto.Message) *any.Any {
	a, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}
	return a
}

func CustomListenerFilters() []*envoy_api_v2_listener.ListenerFilter {
	if ipAllowDenyListenerFilter == nil {
		return []*envoy_api_v2_listener.ListenerFilter{}
	}
	return []*envoy_api_v2_listener.ListenerFilter{ipAllowDenyListenerFilter}
}
