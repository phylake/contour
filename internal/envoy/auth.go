// Copyright © 2018 Heptio
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envoy

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

var (
	// This is the list of ciphers used by Adobe's patched version of Contour
	// for TLS 1.1 and 1.2. TLS 1.3 does not allow cipherlist configuration.
	//
	// Available ciphers are listed in IANA and OpenSSL/BoringSSL documentation:
	// https://www.iana.org/assignments/tls-parameters/tls-parameters.txt
	// https://github.com/google/boringssl/blob/master/include/openssl/tls1.h
	//
	// The list uses OpenSSL/BoringSSL operators:
	// https://commondatastorage.googleapis.com/chromium-boringssl-docs/ssl.h.html#Cipher-suite-configuration
	// Ciphers are ordered by preference; "[CIPHER1|CIPHER2]" means CIPHER1 and
	// CIPHER2 are of equal preference and the client may choose either as
	// they prefer.
	//
	// The Mozilla SSL Configuration Generator's "Old" Configuration is used as
	// a general guideline.
	// https://ssl-config.mozilla.org/#config=old
	//
	// Elliptic Curve (EC) ciphers are preferred as the consensus as of
	// November 2019 is that elliptic curve ciphers are more difficult to
	// attack than prime factorization ciphers.
	// https://blog.cloudflare.com/a-relatively-easy-to-understand-primer-on-elliptic-curve-cryptography
	//
	// RSA ciphers are supported to allow backwards compatibility with old
	// systems. Notably, old Java clients use a cipherlist shipped with the
	// JVM which may be very outdated.
	//
	// DES ciphers are removed as DES is considered insecure and was never
	// intended for protecting secret data.
	//
	// TLS 1.0 and 1.0 extension ciphers are excluded.
	ciphers = []string{
		"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-AES256-GCM-SHA384|ECDHE-ECDSA-CHACHA20-POLY1305]",
		"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-AES256-GCM-SHA384|ECDHE-RSA-CHACHA20-POLY1305]",
		"[DHE-RSA-AES128-GCM-SHA256|DHE-RSA-AES256-GCM-SHA384|DHE-RSA-CHACHA20-POLY1305]",
		"[ECDHE-ECDSA-AES128-SHA256|ECDHE-ECDSA-AES128-SHA|ECDHE-ECDSA-AES256-SHA384|ECDHE-ECDSA-AES256-SHA]",
		"[ECDHE-RSA-AES128-SHA256|ECDHE-RSA-AES128-SHA|ECDHE-RSA-AES256-SHA384|ECDHE-RSA-AES256-SHA]",
		"[DHE-RSA-AES128-SHA256|DHE-RSA-AES256-SHA256]",
		"[AES128-GCM-SHA256|AES256-GCM-SHA384]",
		"[AES128-SHA256|AES256-SHA256]",
	}
)

// UpstreamTLSContext creates an auth.UpstreamTlsContext. By default
// UpstreamTLSContext returns a HTTP/1.1 TLS enabled context. A list of
// additional ALPN protocols can be provided.
func UpstreamTLSContext(ca []byte, subjectName string, alpnProtocols ...string) *auth.UpstreamTlsContext {
	context := &auth.UpstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			AlpnProtocols: alpnProtocols,
		},
	}

	// we have to do explicitly assign the value from validationContext
	// to context.CommonTlsContext.ValidationContextType because the latter
	// is an interface, returning nil from validationContext directly into
	// this field boxes the nil into the unexported type of this grpc OneOf field
	// which causes proto marshaling to explode later on. Not happy Jan.
	vc := validationContext(ca, subjectName)
	if vc != nil {
		context.CommonTlsContext.ValidationContextType = vc
	}

	return context
}

func validationContext(ca []byte, subjectName string) *auth.CommonTlsContext_ValidationContext {
	if len(ca) < 1 {
		// no ca provided, nothing to do
		return nil
	}

	if len(subjectName) < 1 {
		// no subject name provided, nothing to do
		return nil
	}

	return &auth.CommonTlsContext_ValidationContext{
		ValidationContext: &auth.CertificateValidationContext{
			TrustedCa: &core.DataSource{
				// TODO(dfc) update this for SDS
				Specifier: &core.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
			VerifySubjectAltName: []string{subjectName},
		},
	}
}

// DownstreamTLSContext creates a new DownstreamTlsContext.
func DownstreamTLSContext(secretName string, tlsMinProtoVersion auth.TlsParameters_TlsProtocol, alpnProtos ...string) *auth.DownstreamTlsContext {
	return &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: auth.TlsParameters_TLSv1_3,
				CipherSuites:              ciphers,
			},
			TlsCertificateSdsSecretConfigs: []*auth.SdsSecretConfig{{
				Name:      secretName,
				SdsConfig: ConfigSource("contour"),
			}},
			AlpnProtocols: alpnProtos,
		},
	}
}
