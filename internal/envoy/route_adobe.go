package envoy

import (
	"encoding/json"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

func PerFilterConfig(r *dag.Route) (conf map[string]*types.Struct) {
	if r.PerFilterConfig == nil {
		return
	}

	conf = make(map[string]*types.Struct)
	var inInterface map[string]interface{}
	inrec, _ := json.Marshal(r.PerFilterConfig)
	json.Unmarshal(inrec, &inInterface)

	for k, v := range inInterface {
		s := new(types.Struct)
		conf[k] = s

		recurseIface(s, v)
	}
	return
}

// recurseIface is a *types.Value producing function that recurses into nested
// structures
func recurseIface(s *types.Struct, iface interface{}) (ret *types.Value) {

	switch ifaceVal := iface.(type) {
	case int:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{float64(ifaceVal)},
		}
	case int32:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{float64(ifaceVal)},
		}
	case int64:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{float64(ifaceVal)},
		}
	case uint:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{float64(ifaceVal)},
		}
	case uint32:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{float64(ifaceVal)},
		}
	case uint64:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{float64(ifaceVal)},
		}
	case float32:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{float64(ifaceVal)},
		}
	case float64:
		ret = &types.Value{
			Kind: &types.Value_NumberValue{ifaceVal},
		}
	case string:
		ret = &types.Value{
			Kind: &types.Value_StringValue{ifaceVal},
		}
	case bool:
		ret = &types.Value{
			Kind: &types.Value_BoolValue{ifaceVal},
		}
	case map[string]interface{}:
		if s == nil { // will only be true on the initial call
			s = new(types.Struct)
		}
		if s.Fields == nil {
			s.Fields = make(map[string]*types.Value)
		}

		for k, v := range ifaceVal {
			s.Fields[k] = recurseIface(nil, v)
		}

		ret = &types.Value{
			Kind: &types.Value_StructValue{s},
		}
	case []interface{}:
		lv := new(types.ListValue)
		for _, v := range ifaceVal {
			lv.Values = append(lv.Values, recurseIface(nil, v))
		}
		ret = &types.Value{
			Kind: &types.Value_ListValue{lv},
		}
	default:
		ret = &types.Value{
			Kind: &types.Value_NullValue{},
		}
	}
	return
}

func setHashPolicy(r *dag.Route, ra *route.RouteAction) {
	if len(r.HashPolicy) > 0 {
		ra.HashPolicy = make([]*route.RouteAction_HashPolicy, len(r.HashPolicy))
	}

	for i, policy := range r.HashPolicy {
		if policy.Header != nil {
			ra.HashPolicy[i] = &route.RouteAction_HashPolicy{
				PolicySpecifier: &route.RouteAction_HashPolicy_Header_{
					Header: &route.RouteAction_HashPolicy_Header{
						HeaderName: policy.Header.HeaderName,
					},
				},
			}
		} else if policy.Cookie != nil {
			policySpecifier := &route.RouteAction_HashPolicy_Cookie_{
				Cookie: &route.RouteAction_HashPolicy_Cookie{
					Name: policy.Cookie.Name,
					Path: policy.Cookie.Path,
				},
			}
			if policy.Cookie.Ttl != nil {
				policySpecifier.Cookie.Ttl = &policy.Cookie.Ttl.Duration
			}

			ra.HashPolicy[i] = &route.RouteAction_HashPolicy{
				PolicySpecifier: policySpecifier,
			}
		} else if policy.ConnectionProperties != nil {
			ra.HashPolicy[i] = &route.RouteAction_HashPolicy{
				PolicySpecifier: &route.RouteAction_HashPolicy_ConnectionProperties_{
					ConnectionProperties: &route.RouteAction_HashPolicy_ConnectionProperties{
						SourceIp: policy.ConnectionProperties.SourceIp,
					},
				},
			}
		}

		if ra.HashPolicy[i] != nil {
			ra.HashPolicy[i].Terminal = policy.Terminal
		}
	}
}
