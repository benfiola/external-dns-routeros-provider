from typing import Callable, TypeVar

import pytest

SomeCallable = TypeVar("SomeCallable", bound=Callable)


def integration_test(func: SomeCallable) -> SomeCallable:
    return pytest.mark.skip(func)
