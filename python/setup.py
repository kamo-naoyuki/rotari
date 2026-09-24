"""Present only so CI can pass build-time flags (--python-tag, --plat-name)
to bdist_wheel when bundling a platform-specific `rotari` executable.
Project metadata lives in pyproject.toml.
"""

from setuptools import setup

setup()
