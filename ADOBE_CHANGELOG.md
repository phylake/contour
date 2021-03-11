# About

We maintain a set of patches on top of upstream contour. They need to be
semantically versioned as well so we understand, for example, that we're
upgrading a fix version of contour vs. a fix version of our patches.

The scheme is quite simple. The upstream version is first, followed by a hyphen,
followed by our version:

```
C stand for Contour
A stands for Adobe's changes

v{C major}.{C minor}.{C fix}-{A major}.{A minor}.{A fix}-adobe
```

# Log

## v1.5.1-2.16.1-adobe

- cross validate custom host headers against fqdns

## v1.5.1-2.16.0-adobe

- add support for routing on custom host header
- sort endpoints such that IPs in the same subnet are not collocated
- upgrade go to 1.14.15

## v1.5.1-2.15.0-adobe

- fixed issue when using multiple headers for header-based routing
- setStatus now uses the Status subresource
- add response code details to envoy access logs
- upgrade go to 1.14.12

## v1.5.1-2.14.0-adobe

- add support for header-based routing
- upgrade go to 1.14.10

## v1.5.1-2.13.0-adobe

- restore `timeoutPolicy`
- remove restriction on wildcard fqdn
- upgrade go to 1.14.9

## v1.5.1-2.12.1-adobe

- indirectly fix Envoy issue with percent value of 0

## v1.5.1-2.12.0-adobe

- allow dynamic values in custom request/response headers
- upgrade go to 1.14.5

## v1.5.1-2.11.1-adobe

- built from contour v1.5.1

## v1.3.0-2.11.1-adobe

- upgrade go to 1.13.12

## v1.3.0-2.11.0-adobe

- expose RequestHeadersPolicy and ResponseHeadersPolicy config in IngressRoute
- restore `retryPolicy` handling
- restore timeouts via annotation on Ingress resources
- remove annotations and Ingress warnings

## v1.3.0-2.10.1-adobe

- fix grouping issue introduced in v1.3.0-2.10.0-adobe

## v1.3.0-2.10.0-adobe

- optional default TLS server
- fix issue where an http svc would be grouped with an existing tcp_proxy svc

## v1.3.0-2.9.0-adobe

- expose tracing config in IngressRoute
- allow wildcard fqdn

## v1.3.0-2.8.0-adobe

- configure InitialJitter & IntervalJitterPercent
- add support for MaximumProtocolVersion on IngressRoute

## v1.3.0-2.7.0-adobe

- enable panic mode
- log health check failures, when enabled

## v1.3.0-2.6.1-adobe

- built from contour v1.3.0

## v1.1.0-2.6.1-adobe

- fix for rendering protobuf Duration objects when marshaling JSON

## v1.1.0-2.6.0-adobe

- invalidate IRs with idle timeouts <= 0
- conditionally set SO_PRIORITY = 6

## v1.1.0-2.5.0-adobe

- set SO_PRIORITY = 6
- fix some test broken in v1.1.0-2.4.0-adobe

## v1.1.0-2.4.0-adobe

- 1 hour limit on upstream and downstream idle timeouts
- upstream limit moved from Envoy

## v1.1.0-2.3.0-adobe

- set `merge_slashes=true` on HTTP connection manager

## v1.1.0-2.2.3-adobe

- eager load tls secrets only

## v1.1.0-2.2.2-adobe

- throttle logs when waiting for xDS updates

## v1.1.0-2.2.1-adobe

- fix cds sync issue causing the update to potentially wait indefinitely

## v1.1.0-2.2.0-adobe

- protobuf updates
- added warning for future unsupported annotations
- added warning for Route.IdleTimeout deprecation

## v1.1.0-2.1.0-adobe

- built from contour v1.1.0

## v2cc3c38-2.1.0-adobe

- whitelisted `perFilterConfig` fields
- xDS version_info now a hash of the xDS response
- fixed filter chain grouping issue related to min tls version
- built from contour 2cc3c388e9230ab9279d0d5ff5da7a96982f008e

## v0.15.0-2.0.2-adobe

- fix issue where a TLS passthrough would cause contour to crash

## v0.15.0-2.0.1-adobe

- upstream idle timeout was not being configured in Envoy

## v0.15.0-2.0.0-adobe

- disable TLS 1.1

## v0.15.0-1.14.1-adobe

- add gRPC keepalives for Contour -> Envoy
- upgrade go to 1.12.12

## v0.15.0-1.14.0-adobe

- configurable tracing on HTTP connection manager
- configurable TCP keepalives on downstream connections

## v0.15.0-1.13.0-adobe

- remove gzip filter
- remove gRPC-web filter

## v0.15.0-1.12.0-adobe

- built from contour v0.15.0

## v0.14.2-1.12.0-adobe

- re-enable h2
- group filter chains by cert and server names
- better logging when lds is waiting for cds

## v0.14.2-1.11.3-adobe

- remove h2 from ALPN list to fix 404s

## v0.14.2-1.11.2-adobe

- built from contour v0.14.2

## v0.14.0-1.11.2-adobe

- fix issue where the endpoints were not cached properly on start

## v0.14.0-1.11.1-adobe

- fix faulty `validIngressClass` function in upstream impl

## v0.14.0-1.11.0-adobe

- order xDS updates
- skip no-op xDS updates
- discover all resources synchronously on start before sending xDS responses

## v0.14.0-1.10.0-adobe

- health check interval matches K8s probes (200-399)

## v0.14.0-1.9.0-adobe

- built from contour v0.14.0

## v0.13.0-1.9.0-adobe

- add `drain_connections_on_host_removal` to clusters

## v0.13.0-1.8.0-adobe

- configure `envoy.filters.http.health_check_simple` for zero-downtime upgrades

## v0.13.0-1.7.1-adobe

- fix Envoy pegged CPU caused by string parsing

## v0.13.0-1.7.0-adobe

- remove upstream http connection manager idle timeout
- higher default circuit breaking
- three L4 retries and host selections (up from one)

## v0.13.0-rc2-1.6.0-adobe

- built from contour b2b366965f9d5698b82768edf3516ee007326e54

## v0.13.0-rc1-1.6.0-adobe

- disabled configurable retries
- enabled single L4 retry for all virtual hosts

## v0.13.0-rc1-1.5.1-adobe

- fix regression in sticky sessions caused by https://github.com/heptio/contour/pull/1152

## v0.13.0-rc1-1.5.0-adobe

- built from contour c4a296e2717bd362c4f31bfc2760711f4be8265d
- removed http health check filter which apparently caused envoy OOM

## v0.11.0-rc1-1.5.0-adobe

- added http health check to support zero downtime envoy upgrades

## v0.11.0-rc1-1.4.0-adobe

- built from contour 9ebd4791ccf02ed24d1fd8409c0e658561952e3a

## v0.10.2-1.4.0-adobe

- increased max header size filter to match the new envoy hardcoded value
- added idle timeout to service for upstreams

## v0.10.2-1.3.0-adobe

- fixed another xDS protocol issue causing OOM in Envoy

## v0.10.1-1.3.0-adobe

- fixed xDS protocol issue causing OOM in Envoy

## v0.10.0-1.3.0-adobe

- added `envoy.listener.ip_allow_deny` to dynamic listener's filter list
- added `envoy.filters.http.ip_allow_deny` to dynamic listener's filter list
- added `envoy.filters.http.header_size` to dynamic listener's filter list

## v0.10.0-1.2.0-adobe

- upgraded contour

## v0.9.0-1.2.0-adobe

- CRD: `idleTimeout` config
- CRD: `timeout` config

## v0.9.0-1.1.0-adobe

- CRD: `perFilterConfig`
- bootstrap: configurable envoy.filters.http.header_size filter
- bootstrap: configurable envoy.buffer filter
- bootstrap: configurable prometheus mTLS support

## v0.9.0-1.0.0-adobe

- CRD: sticky sessions support
