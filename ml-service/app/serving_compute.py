"""How much of the machine one served prediction is allowed to take (SERVE-01).

numpy's BLAS and scikit-learn's OpenMP runtime each start a thread per core inside every
call they are given, which is the right default for a process doing one big computation and
the wrong one for a service. Two things are wrong with it here. The serving arrays are
small -- a simulate is 20k draws over 22 players -- so those threads cost more to start and
join than they save: on a 12-core box one ``/simulate`` measured **0.455 s** with the
default and **0.176 s** with one thread per library. And the routes now run on the FastAPI
threadpool (this finding's other half), so several requests compute at once and each one
claims all twelve cores: four concurrent simulates measured **7.43 s** together against
**0.42 s** limited -- the machine spending its time on context switches rather than cricket.

**Per request, on the thread that computes, and that detail is the whole design.**
``omp_set_num_threads`` sets OpenMP's thread count for *the calling thread only*; a worker
thread created later reads the initial value from the environment instead. A limit applied
once at import therefore pins the import thread and nothing the service actually computes
on -- measured, and it is not subtle: pinning at import left four concurrent simulates at
**7.59 s**, no better than unpinned. Applying it inside the request, on the threadpool
thread running it, is what reaches the computation.

**Nothing is written to the environment**, which is what keeps this away from training.
``retrain`` and ``evaluate`` run as subprocesses (``python -m ml.xi.retrain``); a child
inherits the environment and cannot inherit a thread-local setting, so scikit-learn's
``HistGradientBoosting`` -- genuinely OpenMP-parallel, and the reason a retrain wants every
core -- keeps all of them. ``tests/test_serving_concurrency.py`` pins both halves: a served
prediction reads one thread, a subprocess of the service reads the machine's default.

The one library this cannot leave alone is OpenBLAS, whose count is process-global rather
than per-thread: while any request holds the limit every thread sees it, and when the last
one exits the process is left at whatever the first one recorded on the way in. In a
process that serves and does nothing else, every value that state can take is one this
module would have chosen anyway.
"""

from __future__ import annotations

import functools
from typing import Any, Callable, List, Optional, TypeVar

import threadpoolctl

#: Threads per numeric library while a request computes. One, for the reasons above.
SERVING_THREADS_PER_LIBRARY = 1

#: Built once and reused: a fresh ``ThreadpoolController`` walks the process's loaded shared
#: libraries, which is not work to repeat per request. Built on first use rather than at
#: import, so that every library the service loads is already there to be found.
_controller: Optional[threadpoolctl.ThreadpoolController] = None

_Served = TypeVar("_Served", bound=Callable[..., Any])


def _numeric_libraries() -> threadpoolctl.ThreadpoolController:
    global _controller
    if _controller is None:
        _controller = threadpoolctl.ThreadpoolController()
    return _controller


def library_thread_counts() -> List[int]:
    """Threads per numeric library as the calling thread sees them -- what the tests read."""
    return [info["num_threads"] for info in threadpoolctl.threadpool_info()]


def single_threaded(function: _Served) -> _Served:
    """Run one serving call with one thread per numeric library.

    The decorator goes on the service function rather than on the route, so that the limit
    covers everything the answer is made of -- the as-of sweep, the feature rows, the
    model's own prediction, the simulator's draws -- and so that a caller reaching
    ``xi_service`` in-process (the tests, a script) is treated like one arriving over HTTP.
    """

    @functools.wraps(function)
    def with_one_thread(*args: Any, **kwargs: Any) -> Any:
        with _numeric_libraries().limit(limits=SERVING_THREADS_PER_LIBRARY):
            return function(*args, **kwargs)

    return with_one_thread  # type: ignore[return-value]
