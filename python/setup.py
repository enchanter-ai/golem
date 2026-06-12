# golem packaging shim. All metadata lives in pyproject.toml; this file exists
# only to force a PLATFORM-specific wheel.
#
# golem ships a prebuilt c-shared library (libgolem.{so,dylib,dll}) loaded at
# runtime via cffi (ABI mode). setuptools sees no compiled *Python* extension,
# so it would tag the wheel pure-Python (py3-none-any) — which cibuildwheel
# rejects, and which would (wrongly) advertise the binary as cross-platform.
# Declaring has_ext_modules() = True makes setuptools emit a platform wheel
# (cp3x-<platform>), the correct tag for a package carrying a native library.
from setuptools import setup
from setuptools.dist import Distribution


class BinaryDistribution(Distribution):
    def has_ext_modules(self) -> bool:
        return True


setup(distclass=BinaryDistribution)
