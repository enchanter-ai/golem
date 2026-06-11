# Third-Party Licenses

golem itself is distributed under the **MIT** license (see [LICENSE](LICENSE)).
It depends on the third-party components below. Each retains its own license;
this file records them and points to the authoritative upstream license text.

## Runtime dependencies (linked into the Go module)

| Dependency | Version | License | Upstream license text |
|------------|---------|---------|-----------------------|
| [`github.com/expr-lang/expr`](https://github.com/expr-lang/expr) | v1.17.8 | **MIT** | https://github.com/expr-lang/expr/blob/master/LICENSE |
| [`github.com/hashicorp/golang-lru/v2`](https://github.com/hashicorp/golang-lru) | v2.0.7 | **MPL-2.0** | https://github.com/hashicorp/golang-lru/blob/main/LICENSE |

### Note on `golang-lru/v2` (MPL-2.0)

`hashicorp/golang-lru/v2` is licensed under the **Mozilla Public License 2.0
(MPL-2.0)** — *not* MIT. MPL-2.0 is a permissive, **file-level** (weak) copyleft
license: it is compatible with distributing golem under MIT, and obligations
(such as making source available for modified MPL-covered files) attach only to
the MPL-covered files themselves, not to golem's own MIT-licensed source. golem
consumes golang-lru/v2 unmodified as a versioned module dependency. See the
upstream [LICENSE](https://github.com/hashicorp/golang-lru/blob/main/LICENSE)
for the full terms.

## Build-time-only tools (not linked into the Go core or the published wheels)

| Tool | License | Role |
|------|---------|------|
| [`gopy`](https://github.com/go-python/gopy) | BSD-3-Clause | generates the Python binding from the Go core |
| [`cibuildwheel`](https://github.com/pypa/cibuildwheel) | BSD-2-Clause | builds the Python wheels |

These tools run during code generation / packaging only and are not part of the
distributed Go module or the compiled binary.
