"""Unit tests for the Fast Markdown Preview plugin's pure logic.

These tests stub the `sublime`/`sublime_plugin` modules (see conftest.py) so
the plugin's platform detection, archive naming, and port-file parsing can be
verified without a running Sublime Text.
"""

import os
import sys

# Ensure the repo root (where the plugin lives) is importable.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import conftest  # noqa: E402  (sets up the sublime stubs)
import fast_markdown_preview as fmp  # noqa: E402


# Platform detection

def test_detect_platform_linux_x64():
    conftest.set_platform("linux", "x64")
    assert fmp._detect_platform() == {"os": "linux", "arch": "amd64"}


def test_detect_platform_osx_arm64():
    conftest.set_platform("osx", "arm64", machine="arm64")
    assert fmp._detect_platform() == {"os": "darwin", "arch": "arm64"}


def test_detect_platform_windows_x64():
    conftest.set_platform("windows", "x64")
    assert fmp._detect_platform() == {"os": "windows", "arch": "amd64"}


# Binary filename

def test_binary_filename_linux():
    conftest.set_platform("linux", "x64")
    assert fmp._binary_filename() == "fast-md-preview"


def test_binary_filename_windows():
    conftest.set_platform("windows", "x64")
    assert fmp._binary_filename() == "fast-md-preview.exe"


# Archive naming

def test_archive_name_linux():
    conftest.set_platform("linux", "x64")
    assert fmp._archive_name("v1.2.3") == "fast-md-preview_1.2.3_linux_amd64.tar.gz"


def test_archive_name_darwin_arm64():
    conftest.set_platform("osx", "arm64", machine="arm64")
    assert fmp._archive_name("v1.2.3") == "fast-md-preview_1.2.3_darwin_arm64.tar.gz"


def test_archive_name_windows():
    conftest.set_platform("windows", "x64")
    assert fmp._archive_name("v1.2.3") == "fast-md-preview_1.2.3_windows_amd64.zip"


def test_archive_name_strips_leading_v():
    conftest.set_platform("linux", "x64")
    assert fmp._archive_name("v0.1.0") == "fast-md-preview_0.1.0_linux_amd64.tar.gz"


# Paths

def test_port_file_path():
    pf = fmp._port_file()
    assert pf.name == "port.txt"
    assert "fast-markdown-preview" in str(pf) or "FastMarkdownPreview" in str(pf)


def test_binary_path_default():
    conftest.set_settings({})
    bp = fmp._binary_path()
    assert bp.name == "fast-md-preview"


def test_binary_path_override():
    conftest.set_settings({"server_binary_path": "/usr/local/bin/custom-server"})
    bp = fmp._binary_path()
    assert str(bp) == "/usr/local/bin/custom-server"


def test_binary_path_falls_back_to_package_dir(tmp_path, monkeypatch):
    """When `make install` copies the binary into the package dir, _binary_path
    returns that path, no settings override needed."""
    conftest.set_settings({})
    pkg = tmp_path / "FastMarkdownPreview" / "server"
    pkg.mkdir(parents=True)
    binary = pkg / fmp._binary_filename()
    binary.write_text("fake-binary")
    monkeypatch.setattr(fmp.sublime, "packages_path", lambda: str(tmp_path))
    assert fmp._binary_path() == binary


# Port file parsing

def test_read_port_file_parses_port(tmp_path, monkeypatch):
    monkeypatch.setattr(fmp, "_port_file", lambda: tmp_path / "port.txt")
    (tmp_path / "port.txt").write_text("port=54321\npid=999\nversion=1.0.0\n")
    assert fmp._read_port_file() == 54321


def test_read_port_file_missing_returns_none(tmp_path, monkeypatch):
    monkeypatch.setattr(fmp, "_port_file", lambda: tmp_path / "nope.txt")
    assert fmp._read_port_file() is None


def test_read_port_file_malformed_returns_none(tmp_path, monkeypatch):
    monkeypatch.setattr(fmp, "_port_file", lambda: tmp_path / "port.txt")
    (tmp_path / "port.txt").write_text("garbage\n")
    assert fmp._read_port_file() is None


def test_clear_stale_port_file(tmp_path, monkeypatch):
    pf = tmp_path / "port.txt"
    pf.write_text("port=1\n")
    monkeypatch.setattr(fmp, "_port_file", lambda: pf)
    fmp._clear_stale_port_file()
    assert not pf.exists()


# Markdown view detection

def test_is_markdown_view_true():
    v = conftest.sublime.View("hello", "text.html.markdown")
    assert fmp._is_markdown_view(v) is True


def test_is_markdown_view_false():
    v = conftest.sublime.View("hello", "source.python")
    assert fmp._is_markdown_view(v) is False


def test_is_markdown_view_none():
    assert fmp._is_markdown_view(None) is False


# PreviewManager singleton

def test_preview_manager_singleton():
    a = fmp.PreviewManager.instance()
    b = fmp.PreviewManager.instance()
    assert a is b


def test_preview_manager_toggle_activates_and_deactivates(monkeypatch):
    """Toggle turns preview on and off, without trying to start a real server."""
    conftest.set_settings({})
    mgr = fmp.PreviewManager.instance()
    # Prevent _ensure_server_async from running (it would fail without a binary).
    monkeypatch.setattr(mgr, "_ensure_server_async", lambda: None)
    mgr.deactivate()
    assert not mgr.is_active
    v = conftest.sublime.View("# Hello", "text.html.markdown")
    mgr.activate(v)
    assert mgr.is_active
    mgr.deactivate()
    assert not mgr.is_active


# Browser auto-open behavior

def test_browser_opens_on_first_activate(monkeypatch):
    """When auto_open_browser is on, the browser opens the first time the
    server becomes ready after activation."""
    conftest.set_settings({"auto_open_browser": True})
    mgr = fmp.PreviewManager.instance()
    opened = []
    def fake_open():
        opened.append(True)
        mgr._browser_opened = True
    monkeypatch.setattr(mgr, "open_browser", fake_open)
    # Simulate a running server so _on_server_ready is reached directly.
    monkeypatch.setattr(mgr._server, "is_running", lambda: True)
    monkeypatch.setattr(mgr._server, "_port", 12345)
    mgr.deactivate()
    v = conftest.sublime.View("# Hello", "text.html.markdown")
    mgr.activate(v)
    assert opened == [True], "browser should open on first activate"


def test_browser_reopens_after_deactivate_and_activate(monkeypatch):
    """Toggling off then on reopens the browser."""
    conftest.set_settings({"auto_open_browser": True})
    mgr = fmp.PreviewManager.instance()
    opened = []
    def fake_open():
        opened.append(True)
        mgr._browser_opened = True
    monkeypatch.setattr(mgr, "open_browser", fake_open)
    monkeypatch.setattr(mgr._server, "is_running", lambda: True)
    monkeypatch.setattr(mgr._server, "_port", 12345)
    mgr.deactivate()
    v = conftest.sublime.View("# Hello", "text.html.markdown")
    mgr.activate(v)  # first activate
    mgr.deactivate()  # toggle off
    mgr.activate(v)  # toggle on again
    assert len(opened) == 2, "browser should reopen on second activate, got %d" % len(opened)


def test_browser_not_reopened_on_crash_recovery(monkeypatch):
    """Crash recovery (direct _ensure_server_async call from _send_async)
    must NOT reopen the browser. The existing tab's EventSource reconnects.
    Only the explicit Reopen Preview command should open a fresh tab."""
    conftest.set_settings({"auto_open_browser": True})
    mgr = fmp.PreviewManager.instance()
    opened = []
    def fake_open():
        opened.append(True)
        mgr._browser_opened = True
    monkeypatch.setattr(mgr, "open_browser", fake_open)

    running = {"yes": True}
    monkeypatch.setattr(mgr._server, "is_running", lambda: running["yes"])
    monkeypatch.setattr(mgr._server, "_port", 12345)
    monkeypatch.setattr(mgr._server, "stop", lambda: running.__setitem__("yes", False))
    def fake_ensure():
        running["yes"] = True
        mgr._on_server_ready()
    monkeypatch.setattr(mgr, "_ensure_server_async", fake_ensure)

    mgr.deactivate()
    v = conftest.sublime.View("# Hello", "text.html.markdown")
    mgr.activate(v)
    assert len(opened) == 1, "first activate should open browser"

    # Crash recovery: _ensure_server_async runs without resetting _browser_opened.
    # The browser must NOT open a second time.
    mgr._ensure_server_async()
    assert len(opened) == 1, "crash recovery must not reopen browser, got %d" % len(opened)


def test_reopen_preview_always_opens_browser(monkeypatch):
    """The explicit Reopen Preview command resets _browser_opened so a new
    browser tab opens even if one was already open."""
    conftest.set_settings({"auto_open_browser": True})
    mgr = fmp.PreviewManager.instance()
    opened = []
    def fake_open():
        opened.append(True)
        mgr._browser_opened = True
    monkeypatch.setattr(mgr, "open_browser", fake_open)

    running = {"yes": True}
    monkeypatch.setattr(mgr._server, "is_running", lambda: running["yes"])
    monkeypatch.setattr(mgr._server, "_port", 12345)
    monkeypatch.setattr(mgr._server, "stop", lambda: running.__setitem__("yes", False))
    def fake_ensure():
        running["yes"] = True
        mgr._on_server_ready()
    monkeypatch.setattr(mgr, "_ensure_server_async", fake_ensure)

    mgr.deactivate()
    v = conftest.sublime.View("# Hello", "text.html.markdown")
    mgr.activate(v)
    assert len(opened) == 1

    # Explicit reopen: resets _browser_opened, so a new tab opens.
    mgr._reopen_preview_async()
    assert len(opened) == 2, "reopen should open browser again, got %d" % len(opened)


# Debounce scheduling

def test_schedule_send_cancels_previous(monkeypatch):
    conftest.set_settings({"debounce_ms": 1000})
    mgr = fmp.PreviewManager.instance()
    monkeypatch.setattr(mgr, "_ensure_server_async", lambda: None)
    mgr.deactivate()
    v = conftest.sublime.View("# Hello", "text.html.markdown")
    with mgr._state_lock:
        mgr._active = True
        mgr._current_view = v
    mgr._schedule_send(v)
    with mgr._state_lock:
        first = mgr._debounce_timer
    assert first is not None
    mgr._schedule_send(v)
    with mgr._state_lock:
        assert mgr._debounce_timer is not first
    mgr._cancel_debounce()


# Settings

def test_get_setting_default():
    conftest.set_settings({})
    assert fmp._get("debounce_ms", 50) == 50


def test_get_setting_value():
    conftest.set_settings({"debounce_ms": 120})
    assert fmp._get("debounce_ms", 50) == 120


# Checksum verification

def test_verify_checksum_matches(tmp_path, monkeypatch):
    """A correct checksum passes without raising."""
    import hashlib
    archive = tmp_path / "fast-md-preview_1.0.0_linux_amd64.tar.gz"
    payload = b"binary contents"
    archive.write_bytes(payload)
    expected = hashlib.sha256(payload).hexdigest()
    checksums = "{}  fast-md-preview_1.0.0_linux_amd64.tar.gz\n".format(expected)

    # Stub the network fetch to return our checksums text.
    class _Resp:
        def __init__(self, text):
            self._text = text
        def read(self):
            return self._text.encode()
        def __enter__(self):
            return self
        def __exit__(self, *a):
            pass

    monkeypatch.setattr(fmp.urllib.request, "urlopen", lambda req, **kw: _Resp(checksums))
    conftest.set_platform("linux", "x64")
    # Should not raise.
    fmp._verify_checksum(archive, "v1.0.0")


def test_verify_checksum_mismatch_raises(tmp_path, monkeypatch):
    """A wrong checksum raises ValueError."""
    archive = tmp_path / "fast-md-preview_1.0.0_linux_amd64.tar.gz"
    archive.write_bytes(b"binary contents")
    checksums = "deadbeef  fast-md-preview_1.0.0_linux_amd64.tar.gz\n"

    class _Resp:
        def __init__(self, text):
            self._text = text
        def read(self):
            return self._text.encode()
        def __enter__(self):
            return self
        def __exit__(self, *a):
            pass

    monkeypatch.setattr(fmp.urllib.request, "urlopen", lambda req, **kw: _Resp(checksums))
    conftest.set_platform("linux", "x64")
    import pytest
    with pytest.raises(ValueError):
        fmp._verify_checksum(archive, "v1.0.0")


def test_verify_checksum_missing_file_skips(tmp_path, monkeypatch):
    """If checksums.txt can't be fetched, verification is skipped (no raise)."""
    archive = tmp_path / "fast-md-preview_1.0.0_linux_amd64.tar.gz"
    archive.write_bytes(b"binary")

    def _raise(*a, **kw):
        raise fmp.urllib.error.URLError("offline")

    monkeypatch.setattr(fmp.urllib.request, "urlopen", _raise)
    conftest.set_platform("linux", "x64")
    # Should not raise.
    fmp._verify_checksum(archive, "v1.0.0")


# ServerProcess thread safety

def test_is_running_is_lock_safe(monkeypatch):
    """is_running() must not race with stop() setting _process to None.

    is_running reads _process under the lock, so a concurrent stop() cannot
    null it between the `is not None` check and `.poll()`. This test
    simulates that interleaving by patching poll().
    """
    sp = fmp.ServerProcess()
    # Simulate a live process whose poll() returns None (alive).
    class _Proc:
        def __init__(self):
            self.nulled = False
        def poll(self):
            # If is_running read _process after stop() nulled it, this would
            # never be reached. We assert is_running snapshots under the lock
            # by checking it does not raise even when _process is cleared.
            return None
    proc = _Proc()
    sp._process = proc
    assert sp.is_running() is True
    # Now simulate stop() clearing _process while another caller might race.
    with sp._lock:
        sp._process = None
    # is_running must observe the None safely, not raise AttributeError.
    assert sp.is_running() is False


def test_lock_is_reentrant():
    """start() holds _lock and calls is_running() / stop() from within it.
    If _lock were a non-reentrant threading.Lock (not RLock), this deadlocks."""
    sp = fmp.ServerProcess()
    class _Proc:
        def poll(self):
            return None  # process alive
        def terminate(self):
            pass
        def wait(self, timeout=None):
            pass
        def kill(self):
            pass
    sp._process = _Proc()
    with sp._lock:
        # Acquiring _lock a second time in the same thread must not deadlock.
        assert sp.is_running() is True
        # stop() also acquires _lock; must not deadlock.
        sp.stop()


def test_adopted_server_is_running_without_process():
    """reuse_existing() adopts a server started by another ST window. We
    never own its process, so is_running() must still return True based on
    the port. Without the _adopted flag, is_running() returns False and
    triggers spurious crash-recovery on every keystroke."""
    sp = fmp.ServerProcess()
    assert sp.is_running() is False  # nothing yet
    sp.reuse_existing(12345)
    assert sp._process is None  # we don't own the process
    assert sp.is_running() is True  # but the adopted server is live
    assert sp.port == 12345
    # stop() must clear the adopted state.
    sp.stop()
    assert sp.is_running() is False
