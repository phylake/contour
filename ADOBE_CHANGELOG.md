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

## v0.9.0-1.1.0-adobe

- CRD: per_filter_config
- bootstrap: configurable envoy.filters.http.header_size filter
- bootstrap: configurable envoy.buffer filter
- bootstrap: configurable prometheus mTLS support

## v0.9.0-1.0.0-adobe

- CRD: sticky sessions support
