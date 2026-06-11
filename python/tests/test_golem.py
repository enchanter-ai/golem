# SPDX-License-Identifier: MIT
# Copyright (c) Enchanter Labs
"""Tests for the golem Python binding.

Two layers:

1. ENVELOPE/EXCEPTION UNIT TESTS (always run): they exercise the pure-Python
   wrapper logic — envelope decoding, type-tag restoration, exception mapping,
   typed accessors — by driving `golem._decode_envelope` and a fake extension
   directly. These need NO gopy build and NO Go toolchain, so they guard the
   most failure-prone part of the binding (the JSON boundary) on every machine.

2. INTEGRATION + GO/PYTHON PARITY TESTS (skipped without the built extension):
   they import the real gopy-generated core and assert the Python results match
   the Go-native results over a shared corpus. They are the acceptance gate
   that runs in CI on a machine WITHOUT a Go toolchain, against a prebuilt
   wheel (per the <acceptance> contract).

The parity corpus below is the single source of truth shared with the Go
parity test (golem/parity_test.go references the identical expression/result
pairs); keep the two in sync so "identical Go and Python results" is verifiable.
"""

from __future__ import annotations

import importlib
import json

import pytest

import golem
from golem import (
    CompileError,
    CostLimitError,
    EvalError,
    ParseError,
    TimeoutError,
    TypeMismatchError,
    UndefinedVariableError,
)


# ---------------------------------------------------------------------------
# Shared corpus — MUST match the Go parity test pair-for-pair.
# (expression, variables, expected_python_result)
# ---------------------------------------------------------------------------
PARITY_CORPUS = [
    ("2 + 3 * (x - 1)", {"x": 5}, 14.0),          # x declared float64 -> float result path
    ("a + b", {"a": 2, "b": 3}, 5),               # int arithmetic -> int
    ("price * 0.9", {"price": 100.0}, 90.0),
    ('status == "active"', {"status": "active"}, True),
    ('score > 0.8 ? "promote" : "hold"', {"score": 0.95}, "promote"),
    ("5 / 2", {}, 2),                              # Go int division truncates -> 2 (silent-zero trap)
    ("5.0 / 2", {}, 2.5),
    ('"foo" + "bar"', {}, "foobar"),
    ('upper("abc")', {}, "ABC"),
    ('"hello" matches "^h"', {}, True),
    ("true and false", {}, False),
    ("not (1 > 2)", {}, True),
    ("nil ?? 7", {}, 7),
    ("let y = 3; y * y", {}, 9),
]


# ---------------------------------------------------------------------------
# Layer 1: pure-Python envelope + exception unit tests (no gopy needed).
# ---------------------------------------------------------------------------
class FakeHandle:
    """A stand-in for the gopy-bound Engine handle. It returns canned envelope
    strings so the wrapper's parsing/exception logic is tested in isolation."""

    def __init__(self, envelope: str):
        self._envelope = envelope
        self.last_call = None

    def EvalJSON(self, src: str, vars_json: str) -> str:  # noqa: N802 - mirrors Go
        self.last_call = (src, vars_json)
        return self._envelope


def _engine_with(envelope: str) -> golem.Engine:
    """Build an Engine whose handle returns a fixed envelope, bypassing gopy."""
    eng = golem.Engine.__new__(golem.Engine)
    eng._handle = FakeHandle(envelope)
    return eng


def test_decode_int_envelope_stays_int():
    assert golem._decode_envelope('{"ok":true,"type":"int","value":14}') == 14
    assert isinstance(golem._decode_envelope('{"ok":true,"type":"int","value":14}'), int)


def test_decode_float_envelope_restores_float_even_when_integral():
    # The crux: JSON renders 2.0 as 2; the "float" tag must restore a Python float.
    result = golem._decode_envelope('{"ok":true,"type":"float","value":2}')
    assert result == 2.0
    assert isinstance(result, float)
    assert not isinstance(result, int) or result.is_integer()
    # And a fractional float round-trips.
    assert golem._decode_envelope('{"ok":true,"type":"float","value":2.5}') == 2.5


def test_decode_bool_string_null():
    assert golem._decode_envelope('{"ok":true,"type":"bool","value":true}') is True
    assert golem._decode_envelope('{"ok":true,"type":"string","value":"hi"}') == "hi"
    assert golem._decode_envelope('{"ok":true,"type":"null","value":null}') is None


@pytest.mark.parametrize(
    "errtype,exc",
    [
        ("ParseError", CompileError),
        ("CompileError", CompileError),
        ("UndefinedVariableError", UndefinedVariableError),
        ("TypeMismatchError", TypeMismatchError),
        ("EvalError", EvalError),
        ("CostLimitError", CostLimitError),
        ("TimeoutError", TimeoutError),
    ],
)
def test_failure_envelope_maps_to_exception(errtype, exc):
    env = json.dumps({"ok": False, "errtype": errtype, "error": "boom"})
    with pytest.raises(exc) as info:
        golem._decode_envelope(env)
    assert "boom" in str(info.value)


def test_parse_error_is_compile_error_alias():
    assert ParseError is CompileError


def test_undefined_variable_is_compile_error_subclass():
    # Lets callers `except CompileError` to catch all compile-time failures.
    assert issubclass(UndefinedVariableError, CompileError)


def test_unknown_errtype_falls_back_to_eval_error():
    env = json.dumps({"ok": False, "errtype": "WeirdError", "error": "x"})
    with pytest.raises(EvalError):
        golem._decode_envelope(env)


def test_malformed_envelope_raises_loudly_not_silent():
    with pytest.raises(EvalError):
        golem._decode_envelope("not json")
    with pytest.raises(EvalError):
        golem._decode_envelope('{"missing":"ok"}')


def test_unknown_type_tag_does_not_guess():
    with pytest.raises(EvalError):
        golem._decode_envelope('{"ok":true,"type":"complex","value":1}')


def test_collection_result_raises_eval_error():
    # The Go boundary encodes a collection result as an EvalError envelope.
    env = (
        '{"ok":false,"errtype":"EvalError",'
        '"error":"collection results unsupported via the Python binding in v1"}'
    )
    with pytest.raises(EvalError) as info:
        golem._decode_envelope(env)
    assert "collection" in str(info.value)


def test_eval_passes_vars_as_json_and_decodes():
    eng = _engine_with('{"ok":true,"type":"int","value":14}')
    assert eng.eval("2 + 3 * (x - 1)", {"x": 5}) == 14
    src, vars_json = eng._handle.last_call
    assert src == "2 + 3 * (x - 1)"
    assert json.loads(vars_json) == {"x": 5}


def test_eval_empty_vars_sends_empty_string():
    eng = _engine_with('{"ok":true,"type":"int","value":2}')
    eng.eval("1 + 1")
    src, vars_json = eng._handle.last_call
    assert vars_json == ""


# Typed accessors over the wrapper (numeric model parity with Go Value.As*).
def test_eval_float_widens_int():
    eng = _engine_with('{"ok":true,"type":"int","value":14}')
    out = eng.eval_float("x")
    assert out == 14.0 and isinstance(out, float)


def test_eval_int_accepts_integral_float_rejects_true_float():
    eng_int = _engine_with('{"ok":true,"type":"float","value":14}')
    assert eng_int.eval_int("x") == 14
    eng_frac = _engine_with('{"ok":true,"type":"float","value":2.5}')
    with pytest.raises(TypeMismatchError):
        eng_frac.eval_int("x")


def test_eval_bool_rejects_non_bool():
    eng = _engine_with('{"ok":true,"type":"int","value":1}')
    with pytest.raises(TypeMismatchError):
        eng.eval_bool("x")


def test_eval_string_rejects_non_string():
    eng = _engine_with('{"ok":true,"type":"int","value":1}')
    with pytest.raises(TypeMismatchError):
        eng.eval_string("x")


def test_eval_float_rejects_bool():
    # bool is an int subclass in Python; must NOT be widened to float.
    eng = _engine_with('{"ok":true,"type":"bool","value":true}')
    with pytest.raises(TypeMismatchError):
        eng.eval_float("x")


def test_program_source_and_reeval():
    eng = _engine_with('{"ok":true,"type":"int","value":14}')
    p = eng.compile("2 + 3 * (x - 1)")
    assert p.source == "2 + 3 * (x - 1)"
    assert p.eval({"x": 5}) == 14


def test_version_single_source():
    assert golem.__version__ == "0.1.0"


# ---------------------------------------------------------------------------
# Wire-form pin: the EXACT JSON bytes `_encode_options` emits must match what
# the Go `NewEngineJSON` boundary parses (engineOptionsJSON in evaljson.go).
# Pure-Python; runs without gopy. These guard the two regressions that broke
# the binding: (F-06) the timeout field MUST be "timeout_ms" not
# "eval_timeout_ms", and (F-07) declared variables MUST be bare typed scalars,
# not {"type","value"} wrapper objects. Go infers the int64-vs-float64 slot
# from the JSON value's own kind, so the int/float distinction must survive in
# the emitted bytes.
# ---------------------------------------------------------------------------
def test_encode_options_timeout_field_is_timeout_ms_not_eval_timeout_ms():
    # F-06: Go's engineOptionsJSON tags the deadline `json:"timeout_ms"`.
    raw = golem._encode_options(
        variables=None,
        strict_vars=True,
        cache_size=None,
        cost_limit=None,
        eval_timeout_ms=50,
    )
    obj = json.loads(raw)
    assert "timeout_ms" in obj, "Go expects the field name 'timeout_ms'"
    assert "eval_timeout_ms" not in obj, "the Python-only name must not leak to Go"
    assert obj["timeout_ms"] == 50


def test_encode_options_variables_are_bare_typed_scalars_not_wrappers():
    # F-07: variables is a bare map[string]any; values must be raw scalars,
    # never {"type":..,"value":..}. The int/float distinction must survive in
    # the bytes so Go normalizeJSONNumber picks int64 vs float64.
    raw = golem._encode_options(
        variables={"x": 0.0, "n": 3, "ok": True, "name": "hi"},
        strict_vars=True,
        cache_size=None,
        cost_limit=None,
        eval_timeout_ms=None,
    )
    obj = json.loads(raw)
    assert obj["variables"] == {"x": 0.0, "n": 3, "ok": True, "name": "hi"}
    # No wrapper objects anywhere in the variables map.
    for v in obj["variables"].values():
        assert not isinstance(v, dict), "schema values must be bare scalars, not wrappers"
    # The float must keep its decimal point in the raw bytes (-> Go float64),
    # and the int must render as an integer literal (-> Go int64).
    assert '"x": 0.0' in raw or '"x":0.0' in raw
    assert '"n": 3' in raw or '"n":3' in raw


def test_encode_options_exact_wire_form_matches_go_test_fixture():
    # The flagship fixture the Go test TestNewEngineJSON_SchemaEvaluatesAndIsLoud
    # parses: {"variables":{"x":0.0}} declares x as a Go float64 slot. Our
    # encoder must produce a superset (it also pins strict_vars) that Go's
    # decoder reads identically: variables.x is the bare number 0.0.
    raw = golem._encode_options(
        variables={"x": 0.0},
        strict_vars=True,
        cache_size=None,
        cost_limit=None,
        eval_timeout_ms=None,
    )
    assert json.loads(raw) == {"strict_vars": True, "variables": {"x": 0.0}}


# ---------------------------------------------------------------------------
# Layer 2: integration + Go/Python parity (require the built gopy extension).
# ---------------------------------------------------------------------------
def _extension_available() -> bool:
    try:
        importlib.import_module("golem._golem")
        return True
    except ImportError:
        return False


requires_extension = pytest.mark.skipif(
    not _extension_available(),
    reason="gopy-generated golem._golem extension not built (run `gopy build`)",
)


@requires_extension
@pytest.mark.parametrize("src,variables,expected", PARITY_CORPUS)
def test_python_matches_expected(src, variables, expected):
    """Each corpus row must evaluate to its expected Python value."""
    e = golem.Engine()
    result = e.eval(src, variables)
    assert result == expected
    # Float-vs-int identity matters for the silent-zero contract.
    assert isinstance(result, type(expected))


@requires_extension
def test_acceptance_float_roundtrips_as_python_float():
    """<acceptance> (a): 2.0 round-trips as a Python float, not int."""
    e = golem.Engine(variables={"x": 0.0})
    out = e.eval("2 + 3 * (x - 1)", {"x": 5})
    assert out == 14.0
    assert isinstance(out, float)


@requires_extension
def test_acceptance_collection_raises_eval_error():
    """<acceptance> (b): a collection-returning expression raises EvalError."""
    e = golem.Engine()
    with pytest.raises(EvalError):
        e.eval("[1, 2, 3]")


@requires_extension
def test_acceptance_cost_limit_wins_when_both_set():
    """<acceptance> (c): with both limits, a cost hit -> CostLimitError."""
    e = golem.Engine(cost_limit=5, eval_timeout_ms=10_000)
    with pytest.raises(CostLimitError):
        e.eval("1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1")


@requires_extension
def test_acceptance_timeout_without_hung_process():
    """<acceptance> (d): a deadline-exceeding eval -> TimeoutError (the caller
    is not blocked past the deadline; the Go side bounds the wait)."""
    # Requires a Go-registered slow function compiled into the core; this calls
    # it by name. If the core lacks it, the test is a no-op skip at the Go side.
    e = golem.Engine(eval_timeout_ms=50)
    with pytest.raises(TimeoutError):
        e.eval("slow_sleep(1000)")  # Go fn that sleeps 1s; raises TimeoutError


@requires_extension
def test_loud_undefined_variable_is_compile_error():
    """LOUD contract: a typo'd declared variable is a compile-time error."""
    e = golem.Engine(variables={"revenue": 0.0})
    with pytest.raises(UndefinedVariableError):
        e.eval("revenu * 2", {"revenue": 100.0})


@requires_extension
def test_lenient_mode_returns_none_for_undefined():
    """WithStrictVars(False) parity: undeclared identifier resolves to None."""
    e = golem.Engine(strict_vars=False)
    assert e.eval("missing") is None
