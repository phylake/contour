package envoy

import (
	"os"
	"strconv"
	"strings"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
)

func validPercent(f float64) bool {
	return f >= 0 && f <= 100
}

func tracing() (config *http.HttpConnectionManager_Tracing) {
	if enabled, err := strconv.ParseBool(os.Getenv("TRACING_ENABLED")); enabled && err == nil {
		config = new(http.HttpConnectionManager_Tracing)

		if strings.ToLower(os.Getenv("TRACING_OPERATION_NAME")) == "egress" {
			config.OperationName = http.HttpConnectionManager_Tracing_EGRESS
		}

		if f, err := strconv.ParseFloat(os.Getenv("TRACING_CLIENT_SAMPLING"), 64); err == nil && validPercent(f) {
			config.ClientSampling = &envoy_type.Percent{Value: f}
		}

		if f, err := strconv.ParseFloat(os.Getenv("TRACING_RANDOM_SAMPLING"), 64); err == nil && validPercent(f) {
			config.RandomSampling = &envoy_type.Percent{Value: f}
		}

		if f, err := strconv.ParseFloat(os.Getenv("TRACING_OVERALL_SAMPLING"), 64); err == nil && validPercent(f) {
			config.OverallSampling = &envoy_type.Percent{Value: f}
		}

		if verbose, err := strconv.ParseBool(os.Getenv("TRACING_VERBOSE")); err == nil {
			config.Verbose = verbose
		}
	}
	return
}

func socketOptions() (opts []*envoy_api_v2_core.SocketOption) {
	const (
		// https://github.com/torvalds/linux/blob/v4.19/arch/ia64/include/uapi/asm/socket.h#L17
		SOL_SOCKET = 1

		// https://github.com/torvalds/linux/blob/v4.19/arch/ia64/include/uapi/asm/socket.h#L29
		SO_KEEPALIVE = 9

		// https://github.com/torvalds/linux/blob/v4.19/arch/ia64/include/uapi/asm/socket.h#L32
		SO_PRIORITY = 12

		// https://github.com/torvalds/linux/blob/v4.19/include/uapi/linux/in.h#L37-L38
		IPPROTO_TCP = 6

		// https://github.com/torvalds/linux/blob/v4.19/include/uapi/linux/tcp.h#L95-L97
		TCP_KEEPIDLE  = 4
		TCP_KEEPINTVL = 5
		TCP_KEEPCNT   = 6
	)

	opts = make([]*envoy_api_v2_core.SocketOption, 0)

	if enabled, err := strconv.ParseBool(os.Getenv("SO_PRIORITY")); enabled && err == nil {
		opts = append(opts, &envoy_api_v2_core.SocketOption{
			Level: SOL_SOCKET,
			Name:  SO_PRIORITY,
			Value: &envoy_api_v2_core.SocketOption_IntValue{6},
			State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
		})
	}

	if enabled, err := strconv.ParseBool(os.Getenv("TCP_KEEPALIVE_ENABLED")); enabled && err == nil {

		opts = append(opts, &envoy_api_v2_core.SocketOption{
			Level: SOL_SOCKET,
			Name:  SO_KEEPALIVE,
			Value: &envoy_api_v2_core.SocketOption_IntValue{1},
			State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
		})

		if i, err := strconv.ParseInt(os.Getenv("TCP_KEEPALIVE_IDLE"), 10, 64); err == nil {
			opts = append(opts, &envoy_api_v2_core.SocketOption{
				Level: IPPROTO_TCP,
				Name:  TCP_KEEPIDLE,
				Value: &envoy_api_v2_core.SocketOption_IntValue{i},
				State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
			})
		}

		if i, err := strconv.ParseInt(os.Getenv("TCP_KEEPALIVE_INTVL"), 10, 64); err == nil {
			opts = append(opts, &envoy_api_v2_core.SocketOption{
				Level: IPPROTO_TCP,
				Name:  TCP_KEEPINTVL,
				Value: &envoy_api_v2_core.SocketOption_IntValue{i},
				State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
			})
		}

		if i, err := strconv.ParseInt(os.Getenv("TCP_KEEPALIVE_CNT"), 10, 64); err == nil {
			opts = append(opts, &envoy_api_v2_core.SocketOption{
				Level: IPPROTO_TCP,
				Name:  TCP_KEEPCNT,
				Value: &envoy_api_v2_core.SocketOption_IntValue{i},
				State: envoy_api_v2_core.SocketOption_STATE_PREBIND,
			})
		}
	}

	if len(opts) == 0 {
		opts = nil
	}

	return
}
