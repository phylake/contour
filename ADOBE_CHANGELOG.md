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

Note: a contour upgrade will not show up in this changelog even if changes were
required to rebuild the patches on top of the changed contour code.

# Log

## 1.1.0

- prometheus mTLS support

## 1.0.0

- sticky session support
