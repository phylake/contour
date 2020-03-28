package v1beta1

import (
	"encoding/json"
	"errors"
	"time"

	ptypes "github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/projectcontour/contour/internal/protobuf"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type Duration struct {
	duration.Duration
}

func (recv Duration) MarshalJSON() ([]byte, error) {
	// convert the protobuf.Duration back to a time.Duration
	timeDuration, err := ptypes.Duration(&recv.Duration)
	// if there was an error with the conversion, return the
	// marshalled duration.Duration instead (old behavior)
	if err != nil {
		return json.Marshal(recv.String())
	}
	return json.Marshal(timeDuration.String())
}

func (recv *Duration) UnmarshalJSON(bs []byte) (err error) {
	var iface interface{}

	if err = json.Unmarshal(bs, &iface); err != nil {
		return
	}

	switch value := iface.(type) {
	case float64:
		recv.Duration = *protobuf.Duration(time.Duration(value))
	case string:
		var d time.Duration
		d, err = time.ParseDuration(value)
		if err == nil {
			recv.Duration = *protobuf.Duration(d)
		}
	default:
		err = errors.New("invalid duration")
	}
	return
}

type HashPolicyHeader struct {
	HeaderName string `json:"headerName"`
}

type HashPolicyCookie struct {
	Name string    `json:"name"`
	Ttl  *Duration `json:"ttl,omitempty"`
	Path string    `json:"path,omitempty"`
}

type HashPolicyConnectionProperties struct {
	SourceIp bool `json:"sourceIp"`
}

type HashPolicy struct {
	Header *HashPolicyHeader `json:"header,omitempty"`

	Cookie *HashPolicyCookie `json:"cookie,omitempty"`

	ConnectionProperties *HashPolicyConnectionProperties `json:"connectionProperties,omitempty"`

	Terminal bool `json:"terminal,omitempty"`
}

type PerFilterConfig struct {
	IpAllowDeny *IpAllowDenyCidrs `json:"envoy.filters.http.ip_allow_deny,omitempty"`
	HeaderSize  *HeaderSize       `json:"envoy.filters.http.header_size,omitempty"`
}

type IpAllowDenyCidrs struct {
	AllowCidrs []Cidr `json:"allow_cidrs,omitempty"`
	DenyCidrs  []Cidr `json:"deny_cidrs,omitempty"`
}

type Cidr struct {
	AddressPrefix *string             `json:"address_prefix,omitempty"`
	PrefixLen     *intstr.IntOrString `json:"prefix_len,omitempty"`
}

type HeaderSize struct {
	HeaderSize struct {
		MaxBytes *int `json:"max_bytes,omitempty"`
	} `json:"header_size,omitempty"`
}
