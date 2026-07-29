"""Test harness for the Fast Markdown Preview plugin.

The plugin imports `sublime` and `sublime_plugin`, which only exist inside the
Sublime Text runtime. These tests stub those modules so the plugin's pure-Python
logic (platform detection, archive naming, port-file parsing, debounce) can be
unit-tested without ST.
"""

import os
import sys
import types
from unittest import mock

# Build a fake `sublime` module before importing the plugin.
sublime = types.ModuleType("sublime")

# Mutable platform/arch the tests can override.
_PLATFORM = {"os": "linux", "arch": "x64", "machine": "x86_64"}


def _platform():
    return _PLATFORM["os"]


def _arch():
    return _PLATFORM["arch"]


_CACHE_PATH = "/tmp/fmp_test_cache"


def _cache_path():
    return _CACHE_PATH


_PACKAGES_PATH = "/tmp/fmp_test_packages"


def _packages_path():
    return _PACKAGES_PATH


sublime.platform = _platform
sublime.arch = _arch
sublime.cache_path = _cache_path
sublime.packages_path = _packages_path

# load_settings returns a dict-like settings object.
_SETTINGS = {}


class _Settings:
    def get(self, key, default=None):
        return _SETTINGS.get(key, default)

    def add_on_change(self, key, callback):
        pass

    def clear_on_change(self, key):
        pass


def _load_settings(name):
    return _Settings()


sublime.load_settings = _load_settings
# Expose the settings type on the stub so the plugin's annotation
# `def _settings() -> sublime.Settings:` can be evaluated at import time on
# Python versions that resolve annotations eagerly (3.8-3.13).
sublime.Settings = _Settings

# Minimal stubs used by the plugin.
sublime.windows = lambda: []
sublime.message_dialog = lambda msg: None
sublime.error_message = lambda msg: None
sublime.status_message = lambda msg: None
sublime.run_command = lambda cmd, args=None: None
sublime.set_timeout_async = lambda fn, delay=0: fn()

# A Region stub.
class _Region:
    def __init__(self, a, b):
        self.a = a
        self.b = b


sublime.Region = _Region

# View stub used by some tests.
class _View:
    def __init__(self, text="", syntax="text.html.markdown"):
        self._text = text
        self._syntax = syntax
        self._status = {}

    def match_selector(self, pos, selector):
        return selector in self._syntax

    def substr(self, region):
        return self._text[region.a:region.b]

    def size(self):
        return len(self._text)

    def buffer_id(self):
        return 1

    def is_valid(self):
        return True

    def set_status(self, key, val):
        self._status[key] = val

    def erase_status(self, key):
        self._status.pop(key, None)

    def file_name(self):
        return "test.md"


sublime.View = _View

sys.modules["sublime"] = sublime

# Fake sublime_plugin module.
sublime_plugin = types.ModuleType("sublime_plugin")


class _TextCommand:
    def __init__(self, view):
        self.view = view

    def run(self, edit, **kwargs):
        raise NotImplementedError


class _EventListener:
    pass


sublime_plugin.TextCommand = _TextCommand
sublime_plugin.EventListener = _EventListener
sys.modules["sublime_plugin"] = sublime_plugin


def set_platform(os_name, arch="x64", machine=None):
    _PLATFORM["os"] = os_name
    _PLATFORM["arch"] = arch
    if machine is None:
        machine = {"x64": "x86_64", "arm64": "arm64"}.get(arch, "x86_64")
    _PLATFORM["machine"] = machine


def set_settings(settings):
    _SETTINGS.clear()
    _SETTINGS.update(settings)


def reset():
    _SETTINGS.clear()
    _PLATFORM["os"] = "linux"
    _PLATFORM["arch"] = "x64"
    _PLATFORM["machine"] = "x86_64"


# Patch platform.machine so the plugin's _detect_platform sees our value.
_real_machine = __import__("platform").machine


def _patched_machine():
    return _PLATFORM["machine"]


import platform as _platform_module
_platform_module.machine = _patched_machine
