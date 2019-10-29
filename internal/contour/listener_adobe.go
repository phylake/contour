package contour

import (
	"encoding/json"
	"os"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/gogo/protobuf/types"
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

var ipAllowDenyListenerFilter *listener.ListenerFilter

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

	structFields := make(map[string]*types.Value)

	if config.AllowCidrs != nil {
		cidrToProto(*config.AllowCidrs, "allow_cidrs", structFields)
	}

	if config.DenyCidrs != nil {
		cidrToProto(*config.DenyCidrs, "deny_cidrs", structFields)
	}

	if len(structFields) > 0 {
		ipAllowDenyListenerFilter = new(listener.ListenerFilter)
		ipAllowDenyListenerFilter.Name = "envoy.listener.ip_allow_deny"
		ipAllowDenyListenerFilter.ConfigType = &listener.ListenerFilter_Config{
			Config: &types.Struct{
				Fields: structFields,
			},
		}
	}
}

func cidrToProto(cidrs []Cidr, key string, structFields map[string]*types.Value) {
	cidrList := &types.ListValue{
		Values: make([]*types.Value, 0),
	}
	structFields[key] = &types.Value{
		Kind: &types.Value_ListValue{
			ListValue: cidrList,
		},
	}

	for _, cidr := range cidrs {
		cidrStruct := &types.Struct{
			Fields: make(map[string]*types.Value),
		}
		cidrStruct.Fields["address_prefix"] = &types.Value{
			Kind: &types.Value_StringValue{
				StringValue: cidr.AddressPrefix,
			},
		}
		cidrStruct.Fields["prefix_len"] = &types.Value{
			Kind: &types.Value_NumberValue{
				NumberValue: cidr.PrefixLen,
			},
		}
		cidrList.Values = append(cidrList.Values, &types.Value{
			Kind: &types.Value_StructValue{
				StructValue: cidrStruct,
			},
		})
	}
}

func CustomListenerFilters() []listener.ListenerFilter {
	if ipAllowDenyListenerFilter == nil {
		return []listener.ListenerFilter{}
	}
	return []listener.ListenerFilter{*ipAllowDenyListenerFilter}
}
