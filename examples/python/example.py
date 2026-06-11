"""Idiomatic use of the golem Python binding.

Install (no Go toolchain or C compiler required — prebuilt wheels):

    pip install enchanter-golem

The Python package is a thin wrapper over the same compiled Go core that powers
the Go API, so results are identical across languages (one source of truth). The
binding crosses dynamic variable maps as a JSON envelope and reconstructs the
correct Python type, raising a mapped exception on failure.

Run it with:

    python example.py
"""

import golem


def main() -> None:
    spec_example()
    bool_and_strings()
    loud_by_default()
    typed_accessors()


def spec_example() -> None:
    # The schema declares x as a float; the int input 5 is normalized to the
    # float slot per golem's numeric model, so the result is 14.
    e = golem.Engine(variables={"x": 0.0})
    result = e.eval("2 + 3 * (x - 1)", {"x": 5})
    print(f"2 + 3 * (x - 1) with x=5  => {result}")  # => 14.0

    # A float stays a float across the JSON envelope (type tag), even though JSON
    # would otherwise render 2.0 as 2.
    assert isinstance(e.eval("4.0 / 2.0", {}), float)


def bool_and_strings() -> None:
    e = golem.Engine(strict_vars=False)  # lenient: user?.email may be absent

    decision = e.eval(
        'status == "active" and score > 0.8 ? "promote" : "hold"',
        {"status": "active", "score": 0.91},
    )
    print(f"ternary decision          => {decision!r}")  # => 'promote'

    email = e.eval('user?.email ?? "no-email"', {"user": None})
    print(f"null-coalesced email      => {email!r}")  # => 'no-email'


def loud_by_default() -> None:
    # LOUD by default: a typo'd top-level variable is a compile-time error, mapped
    # to a same-named Python exception, never a silent zero.
    e = golem.Engine(variables={"revenue": 0.0})
    try:
        e.eval("revenu * 2", {})  # typo: revenu
    except golem.UndefinedVariableError as err:
        print(f"typo caught               => {err}")  # did you mean "revenue"?


def typed_accessors() -> None:
    # Typed accessors mirror the Go surface (EvalBool/EvalFloat/EvalString).
    e = golem.Engine(math_stdlib=True)
    print(f"eval_float sqrt(2500)     => {e.eval_float('sqrt(2500)', {})}")  # 50.0
    print(f"eval_bool 3 > 2           => {e.eval_bool('3 > 2', {})}")        # True
    name_upper = e.eval_string('upper("ada")', {})
    print(f"eval_string upper('ada')  => {name_upper!r}")                    # 'ADA'


if __name__ == "__main__":
    main()
