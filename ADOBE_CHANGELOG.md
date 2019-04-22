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

## v0.10.2-1.3.1-adobe

- increased max header size filter to match the new envoy hardcoded value

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
