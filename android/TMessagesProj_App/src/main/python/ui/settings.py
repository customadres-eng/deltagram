"""Data-only stand-ins for the exteraGram settings widgets."""


class _Widget:
    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)

    def __repr__(self):
        return "%s(%s)" % (type(self).__name__, self.__dict__)


class Header(_Widget):
    pass


class Switch(_Widget):
    pass


class Selector(_Widget):
    pass


class Divider(_Widget):
    pass


class Text(_Widget):
    pass


class Input(_Widget):
    pass


class Custom(_Widget):
    pass


class EditText(_Widget):
    pass


class SimpleSettingFactory:
    pass
