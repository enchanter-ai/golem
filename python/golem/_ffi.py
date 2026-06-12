# SPDX-License-Identifier: MIT
# Copyright (c) Enchanter Labs
"""golem._ffi — cffi loader for golem's c-shared native library.

The evaluation runs in a compiled Go core. That core is exposed to Python as a
small C-shared library (``libgolem``) with a **string-only** boundary — engines
are opaque integer handles and only JSON crosses — built from ``python/capi``
(package ``main``, build tag ``golemcapi``) with::

    go build -tags golemcapi -buildmode=c-shared -o python/golem/libgolem.<ext> ./python/capi

cibuildwheel bundles the built library next to this module, so an installed
wheel needs no Go toolchain or C compiler. This module loads it via cffi in ABI
mode (``dlopen``) — no compilation at import or install time — and exposes three
helpers used by ``golem/__init__.py``: :func:`new_engine`, :func:`eval_json`,
and :func:`free_engine`.

C ABI (see python/capi/main.go):

    long long GolemNewEngine(char *optionsJSON);
    char     *GolemEvalJSON(long long handle, char *src, char *varsJSON);
    void      GolemFreeEngine(long long handle);
    void      GolemFreeString(char *s);
"""
from __future__ import annotations

import os
import sys
import threading

try:
    from cffi import FFI
except ImportError as exc:  # pragma: no cover - cffi is a declared dependency
    raise ImportError(
        "golem requires the 'cffi' package. Install a prebuilt wheel "
        "(`pip install enchanter-golem`), which depends on cffi, or `pip install cffi`."
    ) from exc

_ffi = FFI()
_ffi.cdef(
    """
    long long GolemNewEngine(char *optionsJSON);
    char *GolemEvalJSON(long long handle, char *src, char *varsJSON);
    void GolemFreeEngine(long long handle);
    void GolemFreeString(char *s);
    """
)

# Platform -> bundled library filename (matches the wheels.yml build matrix).
_LIB_NAMES = {
    "linux": "libgolem.so",
    "darwin": "libgolem.dylib",
    "win32": "libgolem.dll",
}

_lib = None
_lock = threading.Lock()


class ExtensionNotBuilt(ImportError):
    """Raised when the native library is not bundled next to this module.

    A prebuilt wheel (``pip install enchanter-golem``) includes the compiled
    library. Building from source requires the ``go build -buildmode=c-shared``
    invocation documented at the top of this module.
    """


def _lib_path() -> str:
    here = os.path.dirname(os.path.abspath(__file__))
    candidates = []
    name = _LIB_NAMES.get(sys.platform)
    if name is not None:
        candidates.append(name)
    # Fall back to any known library name (covers e.g. cygwin/msys reporting).
    candidates.extend(n for n in _LIB_NAMES.values() if n not in candidates)
    for cand in candidates:
        path = os.path.join(here, cand)
        if os.path.exists(path):
            return path
    raise ExtensionNotBuilt(
        f"golem's native library ({candidates[0]}) is not bundled in {here}. "
        "Install a prebuilt wheel (`pip install enchanter-golem`), or build from "
        f"source: `go build -tags golemcapi -buildmode=c-shared -o python/golem/{candidates[0]} ./python/capi`."
    )


def _harden_go_runtime_env() -> None:
    """Disable Go's signal-based async preemption before the runtime starts.

    golem's Go core ships as a ``-buildmode=c-shared`` library loaded into the
    host CPython process. Go's runtime (1.14+) preempts goroutines with an
    asynchronous SIGURG, driven by the background ``sysmon`` goroutine — so it
    fires even though golem's own calls are synchronous and single-threaded.
    Inside a foreign host, that signal can land during a cgo transition and
    intermittently crash the process (observed as a rare SIGSEGV, worst on
    darwin/arm64).

    ``GODEBUG=asyncpreemptoff=1`` restricts preemption to cooperative
    function-call safepoints. golem evaluates short expressions with no long
    uninterruptible loops, so this has no practical scheduling cost. The Go
    runtime reads GODEBUG when the library initializes (at ``dlopen``), so this
    MUST run before :func:`_ffi.dlopen`. We merge rather than overwrite so a
    user's existing GODEBUG settings are preserved.
    """
    godebug = os.environ.get("GODEBUG", "")
    if "asyncpreemptoff=" in godebug:
        return
    os.environ["GODEBUG"] = f"{godebug},asyncpreemptoff=1" if godebug else "asyncpreemptoff=1"


def _load():
    global _lib
    if _lib is None:
        with _lock:
            if _lib is None:
                _harden_go_runtime_env()
                _lib = _ffi.dlopen(_lib_path())
    return _lib


def new_engine(options_json: str) -> int:
    """Construct an engine from a JSON options string; return an opaque handle."""
    lib = _load()
    return int(lib.GolemNewEngine(options_json.encode("utf-8")))


def eval_json(handle: int, src: str, vars_json: str) -> str:
    """Evaluate via the engine handle and return the JSON envelope string.

    The Go side returns a C-malloc'd string; it is copied into Python and freed
    immediately so no native memory leaks across the boundary.
    """
    lib = _load()
    ptr = lib.GolemEvalJSON(handle, src.encode("utf-8"), vars_json.encode("utf-8"))
    try:
        return _ffi.string(ptr).decode("utf-8")
    finally:
        lib.GolemFreeString(ptr)


def free_engine(handle: int) -> None:
    """Release the engine handle (and its compile cache). Best-effort.

    Skipped during interpreter shutdown: ``Engine.__del__`` runs at finalization,
    and calling into the Go c-shared runtime *while it is being torn down* can
    segfault the process (observed as a rare SIGSEGV on darwin/arm64 AFTER the
    test suite had already passed — the crash was in shutdown, not the tests).
    At that point the OS reclaims the handle's memory regardless, so the native
    call is both unsafe and unnecessary.
    """
    if _lib is not None and handle and not sys.is_finalizing():
        _lib.GolemFreeEngine(handle)
