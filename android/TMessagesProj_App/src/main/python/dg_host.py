"""DeltaGram host for exteraGram-style Python plugins.

Loaded through Chaquopy. The Java side calls load_plugin(path, id) and gets back
an empty string on success or a traceback on failure.
"""
import importlib.util
import os
import sys
import traceback

_loaded = {}


def _entry_class(module, module_name):
    from base_plugin import BasePlugin

    for value in vars(module).values():
        if isinstance(value, type) and value.__name__ != "BasePlugin" \
                and getattr(value, "__module__", None) == module_name \
                and issubclass(value, BasePlugin):
            return value
    return None


def load_plugin(path, plugin_id):
    try:
        module_name = "dgplugin_" + "".join(
            ch if ch.isalnum() else "_" for ch in plugin_id)
        spec = importlib.util.spec_from_file_location(module_name, path)
        if spec is None or spec.loader is None:
            return "cannot load " + str(path)
        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)

        cls = _entry_class(module, module_name)
        if cls is None:
            return "no BasePlugin subclass in " + os.path.basename(path)

        cls._dg_id = plugin_id
        cls._dg_dir = os.path.dirname(path)
        instance = cls()
        _loaded[plugin_id] = instance
        instance.on_plugin_load()
        return ""
    except Exception:
        return traceback.format_exc(limit=12)


def unload_plugin(plugin_id):
    instance = _loaded.pop(plugin_id, None)
    if instance is None:
        return ""
    try:
        instance.on_plugin_unload()
        return ""
    except Exception:
        return traceback.format_exc(limit=12)


def plugin_info(plugin_id):
    """Metadata of a loaded plugin, as a JSON string."""
    import json

    instance = _loaded.get(plugin_id)
    if instance is None:
        return "{}"
    return json.dumps({
        "id": plugin_id,
        "class": type(instance).__name__,
        "settings": len(instance.create_settings() or []),
    })
