package envoy

import (
	"testing"

	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/projectcontour/contour/internal/assert"
)

func TestAdobePerFilterConfig(t *testing.T) {
	msi := map[string]interface{}{
		"filter": map[string]interface{}{
			"map": map[string]interface{}{
				"float": 9.0,
				"bool":  true,
			},
			"list": []interface{}{
				"string",
			},
		},
	}
	got := make(map[string]*_struct.Struct)
	for k, v := range msi {
		s := new(_struct.Struct)
		got[k] = s
		recurseIface(s, v)
	}
	want := map[string]*_struct.Struct{
		"filter": {
			Fields: map[string]*_struct.Value{
				"map": {
					Kind: &_struct.Value_StructValue{
						&_struct.Struct{
							Fields: map[string]*_struct.Value{
								"float": {
									Kind: &_struct.Value_NumberValue{9.0},
								},
								"bool": {
									Kind: &_struct.Value_BoolValue{true},
								},
							},
						},
					},
				},
				"list": {
					Kind: &_struct.Value_ListValue{
						&_struct.ListValue{
							Values: []*_struct.Value{
								{
									Kind: &_struct.Value_StringValue{"string"},
								},
							},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, want, got)
}
