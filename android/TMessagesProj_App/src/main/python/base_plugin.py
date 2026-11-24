"""Subset of the exteraGram plugin SDK that DeltaGram can honour today.

Plugins written for exteraGram import ``BasePlugin`` and friends from this
module, so the import must succeed. Hooks are accepted but not installed yet:
``hook_method``/``hook_all_methods`` return False and log a message.
"""
import json
import os
import traceback


class _Logger:
    def __init__(self, plugin_id="plugin"):
        self.plugin_id = plugin_id

    def _log(self, level, message):
        print("[dg-plugin][%s][%s] %s" % (level, self.plugin_id, message))

    def debug(self, message):
        self._log("D", message)

    def info(self, message):
        self._log("I", message)

    def warning(self, message):
        self._log("W", message)

    def error(self, message):
        self._log("E", message)

    def exception(self, message):
        self._log("E", message)
        traceback.print_exc()


class HookStrategy:
    DEFAULT = "default"
    CANCEL = "cancel"
    MODIFY = "modify"
    MODIFY_FINAL = "modify_final"


class HookResult:
    def __init__(self, strategy=HookStrategy.DEFAULT, request=None, response=None,
                 update=None, updates=None, params=None):
        self.strategy = strategy
        self.request = request
        self.response = response
        self.update = update
        self.updates = updates
        self.params = params


class AppEvent:
    START = "start"
    STOP = "stop"
    PAUSE = "pause"
    RESUME = "resume"


class MenuItemType:
    MESSAGE_CONTEXT_MENU = "message"
    DRAWER_MENU = "drawer"
    MAIN_MENU = "main"
    CHAT_ACTION_MENU = "chat_action"
    PROFILE_ACTION_MENU = "profile_action"


class MenuItemData:
    def __init__(self, menu_type=None, text="", on_click=None, icon=None,
                 subtext=None, condition=None, priority=0, item_id=None):
        self.menu_type = menu_type
        self.text = text
        self.on_click = on_click
        self.icon = icon
        self.subtext = subtext
        self.condition = condition
        self.priority = priority
        self.item_id = item_id


class MethodHook:
    """Signature-compatible stand-in; real ART hooking is not wired up yet."""

    def before_hook(self, param):
        return None

    def after_hook(self, param):
        return None


class BasePlugin:
    _dg_id = "plugin"
    _dg_dir = None

    def __init__(self):
        self._id = self._dg_id
        self.logger = _Logger(self._id)
        self._settings = {}
        self._load_settings()

    # ------------------------------------------------------------- settings

    def _settings_path(self):
        base = self._dg_dir or os.path.join(
            os.environ.get("HOME", os.getcwd()), "dg_plugins", self._id)
        try:
            os.makedirs(base, exist_ok=True)
        except Exception:
            pass
        return os.path.join(base, "settings.json")

    def _load_settings(self):
        try:
            with open(self._settings_path(), "r", encoding="utf-8") as handle:
                self._settings = json.load(handle)
        except Exception:
            self._settings = {}

    def _save_settings(self):
        try:
            with open(self._settings_path(), "w", encoding="utf-8") as handle:
                json.dump(self._settings, handle)
        except Exception:
            self.logger.exception("cannot save settings")

    def get_setting(self, key, default=None):
        return self._settings.get(key, default)

    def set_setting(self, key, value, reload_settings=False):
        self._settings[key] = value
        self._save_settings()
        if reload_settings:
            self._load_settings()

    def export_settings(self):
        return dict(self._settings)

    def import_settings(self, settings, reload_settings=True):
        self._settings = dict(settings or {})
        self._save_settings()

    # --------------------------------------------------------------- events

    def on_plugin_load(self):
        pass

    def on_plugin_unload(self):
        pass

    def on_app_event(self, event_type):
        pass

    def create_settings(self):
        return []

    # ---------------------------------------------------------------- hooks

    def _unsupported(self, what):
        self.logger.warning("%s is not supported by this DeltaGram build yet" % what)
        return False

    def add_hook(self, name, match_substring=False, priority=0):
        return self._unsupported("add_hook(%s)" % name)

    def add_on_send_message_hook(self, priority=0):
        return self._unsupported("add_on_send_message_hook")

    def hook_method(self, cls, method, hook):
        return self._unsupported("hook_method(%s)" % method)

    def hook_all_methods(self, cls, method, hook):
        return self._unsupported("hook_all_methods(%s)" % method)

    def add_menu_item(self, item):
        return self._unsupported("add_menu_item(%s)" % getattr(item, "text", ""))

    def remove_menu_item(self, item_id):
        return self._unsupported("remove_menu_item")

    # ---------------------------------------------------------------- misc

    def client(self, account=None):
        raise NotImplementedError("Telegram client access is not exposed yet")
