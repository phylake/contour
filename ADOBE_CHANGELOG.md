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
