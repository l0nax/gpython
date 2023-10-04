import threading_go as _go

class Thread:
    def __init__(self, group=None, target=None, name=None, args=(), kwargs={}, *, daemon=None):
        self._v = _go.Thread_new()

        if target is not None:
            self._v.set_target(target)

        if len(args) > 0:
            self._v.set_args(args)

    def start(self):
        self._v.start()

    def join(self, timeout=None):
        self._v.join(timeout)

    def is_alive(self):
        return self._v.is_alive()
