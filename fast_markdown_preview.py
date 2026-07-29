"""Fast Markdown Preview: live Markdown preview for Sublime Text 4.

Renders Markdown to HTML in an external browser as you type. A companion Go
server (auto-downloaded from GitHub Releases) parses Markdown with goldmark and
streams rendered HTML to the browser via Server-Sent Events.

Architecture:
    Sublime Text (this plugin)
        |  on_modified_async (debounced) + on_activated_async (follow active view)
        |  HTTP POST /update
        v
    Go server (localhost, single binary)
        |  goldmark parse -> HTML
        |  SSE broadcast to all browser tabs
        v
    Browser UI (single HTML page, embedded in the binary)
        |  EventSource -> morphdom DOM diff
"""

import hashlib
import json
import os
import platform
import shutil
import ssl
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import urllib.error
import urllib.request
import webbrowser
import zipfile
from pathlib import Path
from typing import Optional

import sublime
import sublime_plugin

__version__ = "1.0.0"

# Status bar key. Prefixed with spaces for visual padding.
_STATUS_KEY = "fast_markdown_preview"

# How long to wait for the server to report its port on stdout, in seconds.
_SERVER_START_TIMEOUT = 8.0

# Polling interval when waiting for /health after spawn, in seconds.
_HEALTH_POLL_INTERVAL = 0.1


# Settings helpers

def _settings() -> sublime.Settings:
    return sublime.load_settings("FastMarkdownPreview.sublime-settings")


def _get(key: str, default=None):
    return _settings().get(key, default)


# Platform detection

def _detect_platform() -> dict:
    """Map Sublime's platform/arch to the Go target used in release assets."""
    os_map = {"windows": "windows", "osx": "darwin", "linux": "linux"}
    os_name = os_map.get(sublime.platform(), "linux")

    machine = platform.machine().lower()
    arch_map = {
        "x86_64": "amd64", "amd64": "amd64",
        "aarch64": "arm64", "arm64": "arm64",
        "x86": "386", "i386": "386",
    }
    arch = arch_map.get(machine, "amd64")
    return {"os": os_name, "arch": arch}


def _binary_filename() -> str:
    """Filename of the server binary for the current platform."""
    name = "fast-md-preview"
    if sublime.platform() == "windows":
        return name + ".exe"
    return name


def _archive_name(version: str) -> str:
    """Release asset name for the current platform and version (no leading v)."""
    plat = _detect_platform()
    ver = version.lstrip("v")
    ext = ".zip" if plat["os"] == "windows" else ".tar.gz"
    return "fast-md-preview_{}_{}_{}{}".format(ver, plat["os"], plat["arch"], ext)


# Paths

def _cache_dir() -> Path:
    """Directory for the downloaded server binary. Survives package updates."""
    return Path(sublime.cache_path()) / "FastMarkdownPreview" / "server"


def _data_dir() -> Path:
    """Directory for the port file and locks, shared across ST windows."""
    home = Path.home()
    system = platform.system()
    if system == "Linux":
        base = os.environ.get("XDG_DATA_HOME")
        if base:
            return Path(base) / "fast-markdown-preview"
        return home / ".local" / "share" / "fast-markdown-preview"
    if system == "Darwin":
        return home / "Library" / "Application Support" / "fast-markdown-preview"
    # Windows and fallback
    base = os.environ.get("APPDATA")
    if base:
        return Path(base) / "FastMarkdownPreview"
    return home / "AppData" / "Roaming" / "FastMarkdownPreview"


def _port_file() -> Path:
    return _data_dir() / "port.txt"


def _binary_path() -> Path:
    """Resolved path to the server binary, before any download.

    Resolution order:
      1. server_binary_path override (if set)
      2. package dir (where `make install` copies it)
      3. cache dir (auto-download target)
    """
    override = _get("server_binary_path", "")
    if override:
        return Path(override)
    pkg = Path(sublime.packages_path()) / "FastMarkdownPreview" / "server" / _binary_filename()
    if pkg.exists() and pkg.stat().st_size > 0:
        return pkg
    return _cache_dir() / _binary_filename()


def _version_file() -> Path:
    return _cache_dir() / "version.txt"


# File locking (cross-process, cross-window)

class _FileLock:
    """Exclusive file lock using fcntl (Unix) or msvcrt (Windows)."""

    def __init__(self, path: Path):
        self._path = path
        self._fd = None

    def __enter__(self):
        path = str(self._path)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        self._fd = os.open(path, os.O_CREAT | os.O_RDWR, 0o644)
        if platform.system() == "Windows":
            # LK_LOCK blocks until the lock is acquired (with a 10s default
            # timeout), avoiding a busy-wait.
            import msvcrt
            try:
                msvcrt.locking(self._fd, msvcrt.LK_LOCK, 1)
            except OSError:
                pass
        else:
            import fcntl
            fcntl.flock(self._fd, fcntl.LOCK_EX)
        return self

    def __exit__(self, *exc):
        if self._fd is not None:
            if platform.system() == "Windows":
                import msvcrt
                try:
                    msvcrt.locking(self._fd, msvcrt.LK_UNLCK, 1)
                except OSError:
                    pass
            else:
                import fcntl
                fcntl.flock(self._fd, fcntl.LOCK_UN)
            os.close(self._fd)
            self._fd = None


# Binary download

def _ssl_context() -> ssl.SSLContext:
    ctx = ssl.create_default_context()
    ctx.check_hostname = True
    ctx.verify_mode = ssl.CERT_REQUIRED
    return ctx


def _github_api(path: str) -> dict:
    """GET a GitHub API endpoint and return parsed JSON."""
    url = "https://api.github.com" + path
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "FastMarkdownPreview/" + __version__,
    }
    token = os.environ.get("GITHUB_TOKEN", "")
    if token:
        headers["Authorization"] = "token " + token
    req = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(req, timeout=15, context=_ssl_context()) as resp:
        return json.loads(resp.read().decode("utf-8"))


def _fetch_latest_version() -> str:
    """Return the latest release tag (e.g. 'v1.0.0') from GitHub."""
    repo = _get("github_repo", "ipmanlk/fast-markdown-preview")
    data = _github_api("/repos/{}/releases/latest".format(repo))
    return data["tag_name"]


def _download_archive(version: str) -> Path:
    """Download the platform archive for version to a temp file."""
    repo = _get("github_repo", "ipmanlk/fast-markdown-preview")
    asset = _archive_name(version)
    url = "https://github.com/{}/releases/download/{}/{}".format(repo, version, asset)

    tmpdir = Path(tempfile.mkdtemp(prefix="fmp_dl_"))
    dest = tmpdir / asset

    last_err = None
    for attempt in range(1, 4):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "FastMarkdownPreview/" + __version__})
            with urllib.request.urlopen(req, timeout=60, context=_ssl_context()) as resp:
                with open(dest, "wb") as f:
                    shutil.copyfileobj(resp, f)
            return dest
        except urllib.error.URLError as exc:
            last_err = exc
            time.sleep(2 ** attempt)
    shutil.rmtree(tmpdir, ignore_errors=True)
    raise RuntimeError("download failed after 3 attempts: {}".format(last_err))


def _verify_checksum(archive: Path, version: str) -> None:
    """Download checksums.txt and verify the archive. Skips if unavailable.

    GoReleaser publishes a single checksums.txt containing one
    '<sha256>  <filename>' line per asset. We look up the line for our
    archive and compare.
    """
    repo = _get("github_repo", "ipmanlk/fast-markdown-preview")
    asset = _archive_name(version)
    url = "https://github.com/{}/releases/download/{}/checksums.txt".format(repo, version)
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "FastMarkdownPreview/" + __version__})
        with urllib.request.urlopen(req, timeout=15, context=_ssl_context()) as resp:
            text = resp.read().decode("utf-8")
    except urllib.error.URLError:
        print("[FastMarkdownPreview] warning: could not fetch checksums.txt; skipping verification")
        return  # No checksums file available; skip verification.

    expected = None
    for line in text.splitlines():
        parts = line.split()
        if len(parts) >= 2 and parts[1] == asset:
            expected = parts[0]
            break
    if expected is None:
        print("[FastMarkdownPreview] warning: {} not listed in checksums.txt; skipping verification".format(asset))
        return  # Asset not listed; proceed.

    sha = hashlib.sha256()
    with open(archive, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            sha.update(chunk)
    actual = sha.hexdigest()
    if actual != expected:
        raise ValueError("checksum mismatch for {}: expected {}, got {}".format(asset, expected, actual))


def _install_binary(archive: Path, version: str) -> Path:
    """Extract the binary from the archive into the cache dir. Returns its path."""
    cache = _cache_dir()
    cache.mkdir(parents=True, exist_ok=True)
    binary_name = _binary_filename()

    extract_dir = cache / ("extract_" + version.lstrip("v"))
    extract_dir.mkdir(exist_ok=True)

    if archive.name.endswith(".zip"):
        with zipfile.ZipFile(archive, "r") as zf:
            zf.extractall(extract_dir)
    else:
        with tarfile.open(archive, "r:gz") as tf:
            tf.extractall(extract_dir)

    # Find the binary (may be at the root or one level deep).
    candidate = extract_dir / binary_name
    if not candidate.exists():
        for child in extract_dir.rglob(binary_name):
            candidate = child
            break

    if not candidate.exists():
        raise RuntimeError("binary {} not found in archive".format(binary_name))

    final = cache / binary_name
    # Atomic-ish: write to temp then rename.
    tmp = cache / (binary_name + ".tmp")
    if tmp.exists():
        tmp.unlink()
    shutil.move(str(candidate), str(tmp))
    if final.exists():
        final.unlink()
    tmp.rename(final)

    if sublime.platform() != "windows":
        final.chmod(0o755)

    shutil.rmtree(extract_dir, ignore_errors=True)
    shutil.rmtree(archive.parent, ignore_errors=True)

    _version_file().write_text(version, encoding="utf-8")
    return final


def _cached_version() -> Optional[str]:
    vf = _version_file()
    if vf.exists():
        return vf.read_text(encoding="utf-8").strip()
    return None


def ensure_binary() -> Path:
    """Ensure the server binary is present and current. Returns its path."""
    override = _get("server_binary_path", "")
    if override:
        p = Path(override)
        if not p.exists():
            raise FileNotFoundError("custom server_binary_path not found: {}".format(override))
        return p

    # When the binary has been placed in the package directory (for example
    # via `make install`), use it directly. No download or version check needed.
    pkg_binary = Path(sublime.packages_path()) / "FastMarkdownPreview" / "server" / _binary_filename()
    if pkg_binary.exists() and pkg_binary.stat().st_size > 0:
        return pkg_binary

    cache = _cache_dir()
    cache.mkdir(parents=True, exist_ok=True)
    binary = cache / _binary_filename()

    # Try to fetch the latest version; fall back to cached if offline.
    try:
        latest = _fetch_latest_version()
    except Exception:
        if binary.exists():
            return binary
        raise RuntimeError("cannot reach GitHub and no cached binary available")

    if _cached_version() == latest and binary.exists() and binary.stat().st_size > 0:
        return binary

    # Download under a lock so concurrent windows don't race.
    lock_path = _data_dir() / ".download_lock"
    with _FileLock(lock_path):
        # Re-check after acquiring the lock (another window may have installed it).
        if _cached_version() == latest and binary.exists() and binary.stat().st_size > 0:
            return binary
        archive = _download_archive(latest)
        _verify_checksum(archive, latest)
        return _install_binary(archive, latest)


# Server process

class ServerProcess:
    """Manages the Go server subprocess: start, health, stop, crash recovery."""

    def __init__(self):
        self._process: Optional[subprocess.Popen] = None
        self._port: int = 0
        # True when we adopted a server started by another ST window. We
        # never own its process, so is_running() can't poll() it; instead we
        # treat it as running until a health check fails.
        self._adopted = False
        self._lock = threading.RLock()  # reentrant: start() may call is_running()/stop()
        self._reader_thread: Optional[threading.Thread] = None

    @property
    def port(self) -> int:
        with self._lock:
            return self._port

    def is_running(self) -> bool:
        with self._lock:
            if self._adopted:
                # We don't own the process; trust the port until a send or
                # health check proves otherwise. _send_async re-checks health
                # on failure and recovers.
                return self._port != 0
            proc = self._process
        return proc is not None and proc.poll() is None

    def reuse_existing(self, port: int) -> None:
        """Adopt a port belonging to an already-running server (another window)."""
        with self._lock:
            self._port = port
            self._adopted = True

    def health_check(self, timeout: float = 2.0) -> bool:
        if not self._port:
            return False
        try:
            req = urllib.request.Request("http://127.0.0.1:{}/health".format(self._port))
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return resp.status == 200
        except (urllib.error.URLError, OSError):
            return False

    def start(self, binary: Path) -> int:
        """Start the server. Returns the port it is listening on."""
        with self._lock:
            if self.is_running():
                return self._port

            # Fresh start: we own this process.
            self._adopted = False

            if not binary.exists():
                raise FileNotFoundError("server binary not found: {}".format(binary))

            if sublime.platform() != "windows":
                binary.chmod(0o755)

            idle_min = int(_get("idle_timeout_minutes", 5))
            idle_arg = str(idle_min * 60) + "s" if idle_min > 0 else "0"

            args = [
                str(binary),
                "--port", str(int(_get("server_port", 0))),
                "--idle-timeout", idle_arg,
                # Kill the server if no browser connects within 2 minutes.
                # Catches orphaned processes if the browser never opens.
                "--connection-timeout", "2m",
            ]

            # CREATE_NEW_PROCESS_GROUP on Windows so we can send CTRL_BREAK.
            creationflags = 0
            if sublime.platform() == "windows":
                creationflags = subprocess.CREATE_NEW_PROCESS_GROUP

            self._process = subprocess.Popen(
                args,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                stdin=subprocess.DEVNULL,
                creationflags=creationflags,
            )

            # Read the PORT= line from stdout. The server prints it first.
            port = self._read_port_from_stdout(timeout=_SERVER_START_TIMEOUT)
            if not port:
                self._dump_stderr()
                # stop() (not just nulling _process) so we terminate the
                # orphaned subprocess instead of leaking it.
                self.stop()
                raise RuntimeError("server did not report a port within timeout")
            self._port = port

            # Wait for /health to confirm readiness.
            if not self._wait_for_health(timeout=5.0):
                self._dump_stderr()
                self.stop()
                raise RuntimeError("server started but health check failed")
            return self._port

    def _read_port_from_stdout(self, timeout: float) -> int:
        """Read the first stdout line (PORT=<n>) within timeout.

        Uses a background thread + queue so the read cannot block past the
        deadline (a blocking read(1) would ignore the timeout if the server
        is slow to flush).
        """
        import queue
        proc = self._process
        if proc is None or proc.stdout is None:
            return 0

        # The reader thread reads stdout lines until it finds one starting
        # with "PORT=" or the deadline passes. This tolerates log lines the
        # server may print before the port announcement.
        line_q = queue.Queue()
        stop_reading = threading.Event()

        def _reader():
            while not stop_reading.is_set():
                try:
                    line = proc.stdout.readline()
                except Exception as exc:
                    line_q.put(exc)
                    return
                if not line:
                    return  # EOF (process exited)
                text = line.decode("utf-8", errors="replace").strip()
                if text.startswith("PORT="):
                    line_q.put(text)
                    return

        t = threading.Thread(target=_reader, daemon=True)
        t.start()
        try:
            raw = line_q.get(timeout=timeout)
        except queue.Empty:
            stop_reading.set()
            return 0
        stop_reading.set()
        if isinstance(raw, Exception):
            return 0
        if raw.startswith("PORT="):
            try:
                return int(raw[5:])
            except ValueError:
                pass
        return 0

    def _wait_for_health(self, timeout: float) -> bool:
        deadline = time.time() + timeout
        while time.time() < deadline:
            if self.health_check(timeout=1.0):
                return True
            if self._process is not None and self._process.poll() is not None:
                return False
            time.sleep(_HEALTH_POLL_INTERVAL)
        return False

    def _dump_stderr(self):
        """Log server stderr to the ST console if the process died.

        Only reads when the process has already exited, so the pipe is at EOF
        and the read returns promptly. A blocking read on a live process with
        an empty stderr buffer would hang indefinitely.
        """
        proc = self._process
        if proc is None or proc.stderr is None:
            return
        if proc.poll() is None:
            return  # process still alive; do not block on stderr
        try:
            err = proc.stderr.read().decode("utf-8", errors="replace").strip()
            if err:
                print("[FastMarkdownPreview] server stderr:\n" + err)
        except Exception:
            pass

    def stop(self):
        """Stop the server gracefully.

        On Unix, SIGTERM lets the Go server run its signal handler and shut
        down cleanly. On Windows, the process was started in its own process
        group (CREATE_NEW_PROCESS_GROUP), so we send CTRL_BREAK_EVENT which
        the Go runtime translates to SIGINT, allowing graceful shutdown.
        Falls back to a hard kill if it does not exit in time.
        """
        with self._lock:
            proc = self._process
            if proc is None:
                self._port = 0
                return
            if proc.poll() is None:
                try:
                    if sublime.platform() == "windows":
                        import signal
                        proc.send_signal(signal.CTRL_BREAK_EVENT)
                    else:
                        proc.terminate()
                    proc.wait(timeout=5)
                except (subprocess.TimeoutExpired, OSError, ValueError):
                    proc.kill()
                    try:
                        proc.wait(timeout=3)
                    except subprocess.TimeoutExpired:
                        pass
            self._process = None
            self._port = 0
            self._adopted = False

    def send_markdown(self, text: str) -> bool:
        """POST raw markdown to /update. Returns True on success."""
        with self._lock:
            port = self._port
        if not port:
            return False
        url = "http://127.0.0.1:{}/update".format(port)
        data = text.encode("utf-8")
        req = urllib.request.Request(
            url, data=data, method="POST",
            headers={"Content-Type": "text/plain; charset=utf-8"},
        )
        try:
            with urllib.request.urlopen(req, timeout=5) as resp:
                return resp.status == 200
        except (urllib.error.URLError, OSError):
            return False


# Port file discovery (multi-window coordination)

def _read_port_file() -> Optional[int]:
    pf = _port_file()
    if not pf.exists():
        return None
    try:
        for line in pf.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if line.startswith("port="):
                return int(line[5:])
    except (OSError, ValueError):
        pass
    return None


def _check_health_on_port(port: int) -> bool:
    try:
        req = urllib.request.Request("http://127.0.0.1:{}/health".format(port))
        with urllib.request.urlopen(req, timeout=2) as resp:
            return resp.status == 200
    except (urllib.error.URLError, OSError):
        return False


def _clear_stale_port_file():
    pf = _port_file()
    if pf.exists():
        try:
            pf.unlink()
        except OSError:
            pass


# Preview manager (singleton orchestrator)

class PreviewManager:
    """Singleton tying the server, listener, and status bar together.

    The preview model is "follow the active view": a single browser preview
    tracks whichever Markdown view is currently active. When the user switches
    tabs or opens another .md file, the preview updates to show that file.
    """

    _instance: Optional["PreviewManager"] = None
    _instance_lock = threading.Lock()

    def __init__(self):
        self._server = ServerProcess()
        self._active = False  # is preview mode on?
        self._current_view: Optional[sublime.View] = None
        self._debounce_timer: Optional[threading.Timer] = None
        self._send_lock = threading.Lock()
        self._ensure_lock = threading.Lock()
        # Guards _current_view and _debounce_timer, which are touched from
        # both the ST main thread (event handlers) and async worker threads
        # (debounce callbacks, server startup).
        self._state_lock = threading.Lock()
        self._browser_opened = False

    @classmethod
    def instance(cls) -> "PreviewManager":
        if cls._instance is None:
            with cls._instance_lock:
                if cls._instance is None:
                    cls._instance = cls()
        return cls._instance

    # public API

    @property
    def server(self) -> ServerProcess:
        return self._server

    @property
    def is_active(self) -> bool:
        return self._active

    def toggle(self, view: sublime.View):
        if self._active:
            self.deactivate()
        else:
            self.activate(view)

    def activate(self, view: sublime.View):
        """Turn preview on and show the given view."""
        with self._state_lock:
            self._active = True
            self._current_view = view
        self._set_status(view, "starting")
        sublime.set_timeout_async(self._ensure_server_async, 0)

    def deactivate(self):
        """Turn preview off."""
        with self._state_lock:
            self._active = False
            self._current_view = None
            self._cancel_debounce_locked()
            # Reset so the next activate() reopens the browser. Without this,
            # toggling off and back on would not open a browser window.
            self._browser_opened = False
        self._erase_status_all()

    def on_view_modified(self, view: sublime.View):
        """Called (debounced) when a tracked view changes."""
        with self._state_lock:
            active = self._active
        if not active:
            return
        if not _is_markdown_view(view):
            return
        # Update the current view if this is it; otherwise ignore.
        with self._state_lock:
            cur = self._current_view
        if cur is not None and view.buffer_id() == cur.buffer_id():
            self._schedule_send(view)

    def on_view_activated(self, view: sublime.View):
        """Called when the user switches to a view. Follows the active view."""
        if not self._active:
            return
        if not _get("follow_active_view", True):
            return
        if not _is_markdown_view(view):
            return
        with self._state_lock:
            cur = self._current_view
            if cur is not None and view.buffer_id() == cur.buffer_id():
                return
            self._current_view = view
        self._set_status(view, "updating")
        self._schedule_send(view)

    def on_view_closed(self, view: sublime.View):
        with self._state_lock:
            cur = self._current_view
            if cur is not None and view.buffer_id() == cur.buffer_id():
                self._current_view = None

    def open_browser(self):
        if not self._server.is_running() or not self._server.port:
            sublime.message_dialog(
                "FMP: server is not running.\n"
                "Start the preview first with 'Toggle Preview'."
            )
            return
        theme = _get("theme", "auto")
        url = "http://127.0.0.1:{}/?theme={}".format(self._server.port, theme)
        try:
            webbrowser.open(url)
        except Exception:
            sublime.run_command("open_url", {"url": url})
        with self._state_lock:
            self._browser_opened = True

    def reopen_preview(self):
        sublime.set_timeout_async(self._reopen_preview_async, 0)

    # internals

    def _reopen_preview_async(self):
        self._server.stop()
        _clear_stale_port_file()
        # Reset so the browser opens with the new server.
        with self._state_lock:
            self._browser_opened = False
            active = self._active
        if active:
            self._ensure_server_async()

    def _ensure_server_async(self):
        with self._ensure_lock:
            if self._server.is_running():
                self._on_server_ready()
                return
            # Try to discover an existing server (another ST window).
            port = _read_port_file()
            if port and _check_health_on_port(port):
                self._server.reuse_existing(port)
                self._on_server_ready()
                return
            _clear_stale_port_file()
            try:
                binary = ensure_binary()
            except Exception as exc:
                sublime.error_message(
                    "FMP: could not obtain the server binary.\n\n{}".format(exc)
                )
                self.deactivate()
                return
            try:
                self._server.start(binary)
            except Exception as exc:
                print("[FastMarkdownPreview] server start failed: {}".format(exc))
                sublime.error_message(
                    "FMP: server failed to start.\n\n{}".format(exc)
                )
                self.deactivate()
                return
            self._on_server_ready()

    def _on_server_ready(self):
        with self._state_lock:
            view = self._current_view
            browser_opened = self._browser_opened
        if view is not None:
            self._set_status(view, "ready")
            self._schedule_send(view)
        if _get("auto_open_browser", True) and not browser_opened:
            self.open_browser()

    def _schedule_send(self, view: sublime.View):
        """Debounce: cancel any pending send, start a new timer."""
        with self._state_lock:
            self._cancel_debounce_locked()
            delay = max(0, int(_get("debounce_ms", 50))) / 1000.0
            timer = threading.Timer(delay, self._send_async, args=[view])
            timer.daemon = True
            self._debounce_timer = timer
        timer.start()

    def _cancel_debounce(self):
        with self._state_lock:
            self._cancel_debounce_locked()

    def _cancel_debounce_locked(self):
        """Cancel the pending timer. Caller must hold _state_lock."""
        if self._debounce_timer is not None:
            self._debounce_timer.cancel()
            self._debounce_timer = None

    def _send_async(self, view: sublime.View):
        with self._send_lock:
            with self._state_lock:
                active = self._active
            if not active or view is None or not view.is_valid():
                return
            if not self._server.is_running():
                # Server died; recover on ST's async worker (not this timer
                # thread) so main-thread-only APIs stay safe.
                sublime.set_timeout_async(self._ensure_server_async, 0)
                return
            try:
                text = view.substr(sublime.Region(0, view.size()))
            except Exception:
                return
            ok = self._server.send_markdown(text)
            if ok:
                self._set_status(view, "ready")
            else:
                self._set_status(view, "error")
                # Trigger recovery on the async worker.
                sublime.set_timeout_async(self._ensure_server_async, 0)

    # status bar

    def _set_status(self, view: sublime.View, state: str):
        icon = {"ready": "o", "updating": "~", "starting": "...", "error": "x"}.get(state, "?")
        view.set_status(_STATUS_KEY, "  FastMD {}".format(icon))

    def _erase_status_all(self):
        for w in sublime.windows():
            for v in w.views():
                v.erase_status(_STATUS_KEY)


def _is_markdown_view(view: Optional[sublime.View]) -> bool:
    if view is None:
        return False
    try:
        return view.match_selector(0, "text.html.markdown")
    except Exception:
        return False


# Commands

class FmpToggleCommand(sublime_plugin.TextCommand):
    """Toggle the live preview for the current view."""

    def run(self, edit, **kwargs):
        PreviewManager.instance().toggle(self.view)

    def is_enabled(self):
        return _is_markdown_view(self.view)


class FmpReopenCommand(sublime_plugin.TextCommand):
    """Kill the server, restart it, and open a fresh browser tab."""

    def run(self, edit, **kwargs):
        PreviewManager.instance().reopen_preview()
        sublime.status_message("FMP: reopening preview")


# Event listener

class FmpListener(sublime_plugin.EventListener):
    """Listens for edits and view activations to drive the preview."""

    def on_modified_async(self, view: sublime.View):
        PreviewManager.instance().on_view_modified(view)

    def on_activated_async(self, view: sublime.View):
        PreviewManager.instance().on_view_activated(view)

    def on_close(self, view: sublime.View):
        PreviewManager.instance().on_view_closed(view)


# Plugin lifecycle

def plugin_loaded():
    # Ensure the data dir exists for the port file and locks.
    _data_dir().mkdir(parents=True, exist_ok=True)
    # React to settings changes at runtime.
    _settings().add_on_change(_STATUS_KEY, _on_settings_changed)
    print("[FastMarkdownPreview] v{} loaded".format(__version__))


def plugin_unloaded():
    _settings().clear_on_change(_STATUS_KEY)
    mgr = PreviewManager._instance
    if mgr is not None:
        with mgr._state_lock:
            mgr._active = False
            mgr._cancel_debounce_locked()
        # Direct synchronous stop: during shutdown the async worker may not
        # process the scheduled callback before ST exits, orphaning the
        # server. Blocking is acceptable here because ST is already exiting.
        mgr._server.stop()
        _clear_stale_port_file()
        PreviewManager._instance = None


def _on_settings_changed():
    # Currently no live reaction needed beyond debounce/idle values, which are
    # read on each use. Kept as a hook for future settings.
    pass
