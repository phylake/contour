package envoy

import (
	"encoding/json"

	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes/any"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/projectcontour/contour/internal/dag"
)

func PerFilterConfig(r *dag.Route) (conf map[string]*_struct.Struct) {
	if r.PerFilterConfig == nil {
		return
	}

	conf = make(map[string]*_struct.Struct)
	var inInterface map[string]interface{}
	inrec, _ := json.Marshal(r.PerFilterConfig)
	json.Unmarshal(inrec, &inInterface)

	for k, v := range inInterface {
		s := new(_struct.Struct)
		conf[k] = s

		recurseIface(s, v)
	}
	return
}

func TypedPerFilterConfig(r *dag.Route) (conf map[string]*any.Any) {
	if r.PerFilterConfig == nil {
		return
	}

	conf = make(map[string]*any.Any)
	var inInterface map[string]interface{}
	inrec, _ := json.Marshal(r.PerFilterConfig)
	json.Unmarshal(inrec, &inInterface)

	for k, v := range inInterface {
		s := new(_struct.Struct)
		recurseIface(s, v)
		conf[k] = toAny(s)
	}
	return
}

// recurseIface is a *_struct.Value producing function that recurses into nested
// structures
func recurseIface(s *_struct.Struct, iface interface{}) (ret *_struct.Value) {

	switch ifaceVal := iface.(type) {
	case int:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{float64(ifaceVal)},
		}
	case int32:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{float64(ifaceVal)},
		}
	case int64:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{float64(ifaceVal)},
		}
	case uint:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{float64(ifaceVal)},
		}
	case uint32:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{float64(ifaceVal)},
		}
	case uint64:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{float64(ifaceVal)},
		}
	case float32:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{float64(ifaceVal)},
		}
	case float64:
		ret = &_struct.Value{
			Kind: &_struct.Value_NumberValue{ifaceVal},
		}
	case string:
		ret = &_struct.Value{
			Kind: &_struct.Value_StringValue{ifaceVal},
		}
	case bool:
		ret = &_struct.Value{
			Kind: &_struct.Value_BoolValue{ifaceVal},
		}
	case map[string]interface{}:
		if s == nil { // will only be true on the initial call
			s = new(_struct.Struct)
		}
		if s.Fields == nil {
			s.Fields = make(map[string]*_struct.Value)
		}

		for k, v := range ifaceVal {
			s.Fields[k] = recurseIface(nil, v)
		}

		ret = &_struct.Value{
			Kind: &_struct.Value_StructValue{s},
		}
	case []interface{}:
		lv := new(_struct.ListValue)
		for _, v := range ifaceVal {
			lv.Values = append(lv.Values, recurseIface(nil, v))
		}
		ret = &_struct.Value{
			Kind: &_struct.Value_ListValue{lv},
		}
	default:
		ret = &_struct.Value{
			Kind: &_struct.Value_NullValue{},
		}
	}
	return
}

func setHashPolicy(r *dag.Route, ra *envoy_api_v2_route.RouteAction) {
	if len(r.HashPolicy) > 0 {
		ra.HashPolicy = make([]*envoy_api_v2_route.RouteAction_HashPolicy, len(r.HashPolicy))
	}

	for i, policy := range r.HashPolicy {
		if policy.Header != nil {
			ra.HashPolicy[i] = &envoy_api_v2_route.RouteAction_HashPolicy{
				PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Header_{
					Header: &envoy_api_v2_route.RouteAction_HashPolicy_Header{
						HeaderName: policy.Header.HeaderName,
					},
				},
			}
		} else if policy.Cookie != nil {
			policySpecifier := &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
					Name: policy.Cookie.Name,
					Path: policy.Cookie.Path,
				},
			}
			if policy.Cookie.Ttl != nil {
				policySpecifier.Cookie.Ttl = &policy.Cookie.Ttl.Duration
			}

			ra.HashPolicy[i] = &envoy_api_v2_route.RouteAction_HashPolicy{
				PolicySpecifier: policySpecifier,
			}
		} else if policy.ConnectionProperties != nil {
			ra.HashPolicy[i] = &envoy_api_v2_route.RouteAction_HashPolicy{
				PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_ConnectionProperties_{
					ConnectionProperties: &envoy_api_v2_route.RouteAction_HashPolicy_ConnectionProperties{
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
