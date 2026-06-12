# SPDX-License-Identifier: MIT
# Copyright (c) Enchanter Labs
"""golem — a Pythonic skin over the Go expression-evaluation core.

golem lets non-engineers author expressions like ``2 + 3 * (x - 1)`` that
evaluate safely and never fail silently. The evaluation itself runs entirely in
the compiled Go core (a thin wrapper around github.com/expr-lang/expr); this
package is a thin, hand-written Python wrapper over the gopy-generated handle
module. It mirrors the Go surface:

    >>> import golem
    >>> e = golem.Engine(variables={"x": 0.0})   # x declared float64 (LOUD)
    >>> e.eval("2 + 3 * (x - 1)", {"x": 5})       # int 5 coerced to the schema
    14.0

Tier-2 binding note (honest numbers): the Go-native API is the production hot
path (millions/sec, thread-safe). This Python binding is native-fast and
trivially installable, but pays a gopy + JSON cost per call — it is the
authoring/product layer, not the RTB hot path.

------------------------------------------------------------------------------
Boundary contract (see golem/_ffi.py for the full rationale)
------------------------------------------------------------------------------
The dynamic path crosses the gopy boundary as a JSON ENVELOPE rather than a Go
``(value, error)`` pair, because gopy's value/error -> exception mapping is
buggy for ``(string, error)`` (go-python/gopy issue #254). Every outcome,
success or failure, is encoded inside the envelope string returned by the Go
``Engine.EvalJSON``. This wrapper parses it, restores the Python type from the
``type`` tag (so a float ``2.0`` stays a Python ``float``, not an ``int``), and
raises the mapped exception when ``ok`` is false.

v1 limitations (documented; v2 roadmap):
  * The Python ``Engine`` does NOT accept Python callables as custom functions.
    Only Go-registered functions compiled into the shared library are callable
    by name from expression text.
  * Collection results (slice/map) are not supported via the binding and raise
    ``EvalError``; the Go-native API returns them.
"""

from __future__ import annotations

import json
from typing import Any, Dict, Mapping, Optional

from . import _ffi

__all__ = [
    "Engine",
    "Program",
    "GolemError",
    "CompileError",
    "ParseError",
    "UndefinedVariableError",
    "TypeMismatchError",
    "EvalError",
    "CostLimitError",
    "TimeoutError",
    "OverflowError",
    "__version__",
]

# Single source of truth for the version. pyproject.toml reads this same value
# (tool.setuptools.dynamic / attr = "golem.__version__"), so there is never a
# second hardcoded version string. It matches the Go release tag (v0.1.0).
__version__ = "0.1.0"


# ---------------------------------------------------------------------------
# Exception hierarchy — one class per Go typed error, same names.
# ---------------------------------------------------------------------------
class GolemError(Exception):
    """Base class for every error golem raises."""


class CompileError(GolemError):
    """An expression failed to parse or type-check (compile time).

    The Go core's ``ParseError`` maps here. ``ParseError`` is exported as an
    alias so callers may catch either name.
    """


# The Go core names its malformed-expression error ``ParseError``; in Python we
# surface compile-time failures under ``CompileError`` (the name the public API
# documents) and keep ``ParseError`` as an alias so the envelope's
# "ParseError" errtype and user `except ParseError` both work.
ParseError = CompileError


class UndefinedVariableError(CompileError):
    """A strict-mode expression referenced an undeclared top-level identifier.

    Compile-time. The Go message carries the offending name and, when one is
    close in the declared schema, a "did you mean" suggestion.
    """


class TypeMismatchError(GolemError):
    """A type-check or typed-accessor mismatch (compile time or input coercion)."""


class EvalError(GolemError):
    """A runtime failure during evaluation (including a recovered panic, a
    NaN/Inf result, or an unsupported collection result via the binding)."""


class CostLimitError(GolemError):
    """The expression exceeded the configured AST node-visit budget."""


class TimeoutError(GolemError):  # noqa: A001 - intentional golem-scoped name
    """The evaluation exceeded the configured wall-clock deadline.

    Named to mirror the Go ``TimeoutError``; distinct from the builtin
    ``TimeoutError`` (callers reference it as ``golem.TimeoutError``)."""


class OverflowError(GolemError):  # noqa: A001 - intentional golem-scoped name
    """An integer operation whose result is not representable in int64.

    Mirrors the Go ``OverflowError`` (e.g. math.MinInt64 / -1, or an unsigned
    result above int64 range); distinct from the builtin ``OverflowError``
    (callers reference it as ``golem.OverflowError``)."""


# Maps the envelope's ``errtype`` string to the Python exception class. The keys
# are exactly the Go typed-error names emitted by ``evaljson.go``'s ``errType``.
# "ParseError" is included because the Go core emits that name; it resolves to
# CompileError (ParseError is an alias of it).
_ERRTYPE_TO_EXC: Dict[str, type] = {
    "ParseError": CompileError,
    "CompileError": CompileError,
    "UndefinedVariableError": UndefinedVariableError,
    "TypeMismatchError": TypeMismatchError,
    "EvalError": EvalError,
    "CostLimitError": CostLimitError,
    "TimeoutError": TimeoutError,
    "OverflowError": OverflowError,
}


def _raise_from_envelope(env: Mapping[str, Any]) -> None:
    """Raise the mapped exception for a failure envelope (``ok=false``)."""
    errtype = env.get("errtype", "EvalError")
    message = env.get("error", "golem: evaluation failed")
    exc_cls = _ERRTYPE_TO_EXC.get(errtype, EvalError)
    raise exc_cls(message)


def _decode_envelope(raw: str) -> Any:
    """Parse a JSON envelope string from the Go boundary and return the value.

    On a success envelope, the result's Python type is restored from the
    ``type`` tag so it survives JSON's lossy number rendering:

      * "int"    -> int
      * "float"  -> float  (a "float" tagged 2 becomes 2.0, NOT int 2)
      * "bool"   -> bool
      * "string" -> str
      * "null"   -> None

    On a failure envelope, the mapped exception is raised.
    """
    try:
        env = json.loads(raw)
    except (ValueError, TypeError) as exc:
        # The boundary is contractually a valid envelope; a parse failure here
        # means the extension returned something unexpected. Surface it loudly
        # rather than returning a silently-wrong value.
        raise EvalError(
            f"golem: malformed envelope from the Go boundary: {raw!r}"
        ) from exc

    if not isinstance(env, dict) or "ok" not in env:
        raise EvalError(f"golem: malformed envelope from the Go boundary: {raw!r}")

    if not env.get("ok", False):
        _raise_from_envelope(env)

    tag = env.get("type")
    value = env.get("value")

    if tag == "null":
        return None
    if tag == "bool":
        return bool(value)
    if tag == "string":
        return str(value)
    if tag == "int":
        # JSON already yields a Python int here; normalize defensively.
        return int(value)
    if tag == "float":
        # CRITICAL: restore float even when JSON rendered an integral value as
        # an int (e.g. 14 or 2 with type "float" must become 14.0 / 2.0).
        return float(value)

    # Unknown tag: the envelope contract was violated. Do not guess a type.
    raise EvalError(f"golem: unknown result type tag {tag!r} in envelope {raw!r}")


class Program:
    """A compiled, immutable expression bound to an :class:`Engine`.

    Because golem's Python binding crosses dynamic data as JSON envelopes via
    the Engine, a ``Program`` is a thin convenience that re-evaluates its source
    through the owning Engine's cache (the Go core caches the compiled program
    per source, so this does not recompile). It mirrors the Go ``Program``
    typed-accessor surface.
    """

    __slots__ = ("_engine", "_source")

    def __init__(self, engine: "Engine", source: str) -> None:
        self._engine = engine
        self._source = source

    @property
    def source(self) -> str:
        """The original expression text."""
        return self._source

    def eval(self, variables: Optional[Mapping[str, Any]] = None) -> Any:
        """Evaluate and return the native Python result."""
        return self._engine.eval(self._source, variables)

    def eval_bool(self, variables: Optional[Mapping[str, Any]] = None) -> bool:
        """Evaluate and return a ``bool`` (raises ``TypeMismatchError`` otherwise)."""
        result = self.eval(variables)
        if not isinstance(result, bool):
            raise TypeMismatchError(
                f"golem: type mismatch: expected bool, got {_py_type_name(result)}"
            )
        return result

    def eval_float(self, variables: Optional[Mapping[str, Any]] = None) -> float:
        """Evaluate and return a ``float``, widening an ``int`` result per the
        numeric model (the only implicit result coercion)."""
        result = self.eval(variables)
        # bool is a subclass of int in Python; exclude it explicitly.
        if isinstance(result, bool) or not isinstance(result, (int, float)):
            raise TypeMismatchError(
                f"golem: type mismatch: expected float, got {_py_type_name(result)}"
            )
        return float(result)

    def eval_int(self, variables: Optional[Mapping[str, Any]] = None) -> int:
        """Evaluate and return an ``int``. A true (non-integral) float raises
        ``TypeMismatchError``; an integral float (14.0) is accepted."""
        result = self.eval(variables)
        if isinstance(result, bool):
            raise TypeMismatchError("golem: type mismatch: expected int, got bool")
        if isinstance(result, int):
            return result
        if isinstance(result, float):
            if result.is_integer():
                return int(result)
            raise TypeMismatchError(
                f"golem: type mismatch: expected int, got float (non-integral value {result})"
            )
        raise TypeMismatchError(
            f"golem: type mismatch: expected int, got {_py_type_name(result)}"
        )

    def eval_string(self, variables: Optional[Mapping[str, Any]] = None) -> str:
        """Evaluate and return a ``str`` (raises ``TypeMismatchError`` otherwise)."""
        result = self.eval(variables)
        if not isinstance(result, str):
            raise TypeMismatchError(
                f"golem: type mismatch: expected string, got {_py_type_name(result)}"
            )
        return result

    def __repr__(self) -> str:  # pragma: no cover - cosmetic
        return f"golem.Program({self._source!r})"


class Engine:
    """A reusable, thread-safe expression engine backed by the Go core.

    Mirrors the Go ``golem.New`` option surface as keyword arguments:

      * ``variables``   — declared variable schema (LOUD: an undeclared
        top-level identifier is a compile-time ``UndefinedVariableError``).
        Python values map to Go schema types: ``float`` -> float64,
        ``int`` -> int64, ``bool`` -> bool, ``str`` -> string.
      * ``strict_vars`` — default ``True``. ``False`` enables lenient mode
        (undeclared variables resolve to nil/None instead of failing).
      * ``cache_size``  — per-Engine LRU compiled-program cache capacity.
      * ``cost_limit``  — AST node-visit budget; exceeding it raises
        ``CostLimitError``. The preferred production guard.
      * ``eval_timeout_ms`` — per-eval wall-clock deadline in milliseconds;
        exceeding it raises ``TimeoutError``.

    v1 limitation: custom functions cannot be supplied from Python — only
    Go-registered functions compiled into the shared library are callable
    (see the module docstring).
    """

    __slots__ = ("_handle",)

    def __init__(
        self,
        variables: Optional[Mapping[str, Any]] = None,
        *,
        strict_vars: bool = True,
        cache_size: Optional[int] = None,
        cost_limit: Optional[int] = None,
        eval_timeout_ms: Optional[int] = None,
    ) -> None:
        # The engine configuration crosses the C boundary as a JSON string via
        # GolemNewEngine / NewEngineJSON (see golem/_ffi.py). An options-only or
        # empty string yields the default LOUD engine. The result is an opaque
        # integer handle into the Go-side engine registry.
        options_json = _encode_options(
            variables=variables,
            strict_vars=strict_vars,
            cache_size=cache_size,
            cost_limit=cost_limit,
            eval_timeout_ms=eval_timeout_ms,
        )
        self._handle = _ffi.new_engine(options_json)

    def compile(self, source: str) -> Program:
        """Return a :class:`Program` for ``source``.

        The Go core compiles and caches per source; an invalid expression
        raises a compile-time error on first evaluation. We validate eagerly by
        evaluating against an empty variable set is NOT done here (it could
        falsely trip undefined-variable checks for valid expressions needing
        inputs); compilation errors surface on the first ``eval``.
        """
        return Program(self, source)

    def eval(
        self, source: str, variables: Optional[Mapping[str, Any]] = None
    ) -> Any:
        """Compile (cached) and evaluate ``source`` against ``variables``.

        Returns a native Python value (``int``/``float``/``bool``/``str``/
        ``None``) restored from the envelope ``type`` tag. Raises the mapped
        golem exception on any failure.
        """
        vars_json = "" if not variables else json.dumps(dict(variables))
        raw = _ffi.eval_json(self._handle, source, vars_json)
        return _decode_envelope(raw)

    def eval_bool(
        self, source: str, variables: Optional[Mapping[str, Any]] = None
    ) -> bool:
        return self.compile(source).eval_bool(variables)

    def eval_float(
        self, source: str, variables: Optional[Mapping[str, Any]] = None
    ) -> float:
        return self.compile(source).eval_float(variables)

    def eval_int(
        self, source: str, variables: Optional[Mapping[str, Any]] = None
    ) -> int:
        return self.compile(source).eval_int(variables)

    def eval_string(
        self, source: str, variables: Optional[Mapping[str, Any]] = None
    ) -> str:
        return self.compile(source).eval_string(variables)

    def __del__(self) -> None:  # release the Go-side engine handle + its cache
        handle = getattr(self, "_handle", 0)
        if handle:
            try:
                _ffi.free_engine(handle)
            except Exception:  # pragma: no cover - best-effort cleanup
                pass

    def __repr__(self) -> str:  # pragma: no cover - cosmetic
        return "golem.Engine()"


def _encode_options(
    *,
    variables: Optional[Mapping[str, Any]],
    strict_vars: bool,
    cache_size: Optional[int],
    cost_limit: Optional[int],
    eval_timeout_ms: Optional[int],
) -> str:
    """Encode Engine configuration as the JSON the Go ``NewEngineJSON`` boundary
    expects. Only set fields are included so the Go side can apply its defaults.

    Wire-form is dictated by Go's ``engineOptionsJSON`` struct (evaljson.go): the
    field is ``timeout_ms`` (NOT ``eval_timeout_ms``), and ``variables`` is a
    bare ``map[string]any`` — each declared variable's value is the raw typed
    scalar, NOT a ``{"type","value"}`` wrapper. Go rebuilds the correct
    ``expr.Env`` slot type from the JSON value itself: ``json.Decoder`` with
    ``UseNumber`` + ``normalizeJSONNumber`` turns an integral number into int64
    and a fractional number into float64, so a Python ``float`` (rendered by
    ``json.dumps`` as ``0.0``) declares a Go float64 slot and a Python ``int``
    declares an int64 slot; ``bool``/``str`` map directly.
    """
    opts: Dict[str, Any] = {"strict_vars": strict_vars}
    if variables is not None:
        opts["variables"] = {k: _schema_slot(v) for k, v in variables.items()}
    if cache_size is not None:
        opts["cache_size"] = cache_size
    if cost_limit is not None:
        opts["cost_limit"] = cost_limit
    if eval_timeout_ms is not None:
        # Go's engineOptionsJSON tags this field "timeout_ms" (int64 ms).
        opts["timeout_ms"] = eval_timeout_ms
    return json.dumps(opts)


def _schema_slot(value: Any) -> Any:
    """Return the BARE typed scalar Go's ``NewEngineJSON`` expects for a declared
    variable. Go infers the slot type from the JSON value's own type
    (int64 vs float64 vs bool vs string) — no ``{"type","value"}`` wrapper. The
    int64-vs-float64 distinction is preserved by ``json.dumps``: a Python
    ``float`` renders with a decimal point (``0.0``), which Go normalizes to
    float64; a Python ``int`` renders as an integer literal, normalized to int64.
    Order matters: ``bool`` is a subclass of ``int`` and must be tested first."""
    if isinstance(value, bool):
        return value
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return value
    if isinstance(value, str):
        return value
    raise TypeMismatchError(
        f"golem: unsupported schema type for declared variable: {_py_type_name(value)}"
    )


def _py_type_name(value: Any) -> str:
    """A stable, golem-flavored type label for Python-side mismatch messages."""
    if value is None:
        return "nil"
    if isinstance(value, bool):
        return "bool"
    if isinstance(value, str):
        return "string"
    if isinstance(value, float):
        return "float"
    if isinstance(value, int):
        return "int"
    return type(value).__name__
