#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) Enchanter Labs
"""Verify a PyPI-installed ``enchanter-golem`` from a real Python user's seat.

Run it right after ``pip install enchanter-golem``::

    python scripts/verify_install.py

It touches only the public, Pythonic API — the same way a data scientist would
in a notebook, with no Go toolchain in sight — and asserts golem's core promises:
arithmetic and variable lookup, booleans/ternary, strings, and **LOUD by
default** (a typo'd variable is an error, never a silent ``0``). Exits non-zero
on any failure so both CI and a human get an unambiguous pass/fail.
"""
from __future__ import annotations

import golem


def main() -> int:
    # One engine, a declared variable schema — the typical entry point.
    engine = golem.Engine(variables={"x": 0.0, "name": ""})

    # (label, actual, expected) for the value checks.
    checks = [
        ("arithmetic  2 + 3 * (x - 1) @ x=5", engine.eval("2 + 3 * (x - 1)", {"x": 5}), 14.0),
        ("ternary     x > 3 ? big : small", engine.eval('x > 3 ? "big" : "small"', {"x": 5}), "big"),
        ("strings     \"hi \" + name", engine.eval('"hi " + name', {"name": "there"}), "hi there"),
    ]

    failures = 0
    for label, got, want in checks:
        ok = got == want
        failures += not ok
        print(f"  [{'ok' if ok else 'FAIL'}] {label}  ->  {got!r}" + ("" if ok else f"  (want {want!r})"))

    # LOUD by default: an undeclared / typo'd variable must RAISE, never return 0.
    try:
        engine.eval("xx + 1", {})
    except Exception as exc:  # noqa: BLE001 — any golem error proves the loud contract
        print(f"  [ok] LOUD        undeclared 'xx' raised {type(exc).__name__}")
    else:
        print("  [FAIL] LOUD        undeclared 'xx' did NOT raise (silent-zero regression)")
        failures += 1

    if failures:
        print(f"\nVERIFY FAILED - {failures} check(s) failed on golem {golem.__version__}")
        return 1
    print(f"\nVERIFY OK - golem {golem.__version__} installed from PyPI and behaving correctly")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
