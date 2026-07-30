"""End-to-end integration test: real Go server + plugin's HTTP client.

This builds the Go server, starts it, and exercises the plugin's ServerProcess
and send_markdown logic against the live server. It verifies the full pipeline
works: port discovery, health check, markdown POST, and SSE delivery.
"""

import os
import shutil
import socket
import subprocess
import sys
import time

import pytest

# Skip the entire module if Go is not installed (e.g. on CI without Go).
_GO = shutil.which("go")
pytestmark = pytest.mark.skipif(_GO is None, reason="Go toolchain not available")

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SERVER_DIR = os.path.join(REPO_ROOT, "server")
BINARY = os.path.join(REPO_ROOT, "tests", "fmp-server-test")

# Ensure the sublime stubs are loaded before importing the plugin.
sys.path.insert(0, os.path.join(REPO_ROOT, "tests"))
import conftest  # noqa: E402
import fast_markdown_preview as fmp  # noqa: E402


def _free_port():
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


@pytest.fixture(scope="module")
def server_binary():
    """Build the Go server once for the whole module."""
    subprocess.run(
        ["go", "build", "-o", BINARY, "."],
        cwd=SERVER_DIR, check=True, capture_output=True,
    )
    yield BINARY
    if os.path.exists(BINARY):
        os.remove(BINARY)


@pytest.fixture
def running_server(server_binary, tmp_path, monkeypatch):
    """Start a fresh server on a free port for each test."""
    port = _free_port()
    proc = subprocess.Popen(
        [server_binary, "--port", str(port), "--idle-timeout", "0",
         "--data-dir", str(tmp_path)],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    # Wait for the PORT= line.
    line = proc.stdout.readline().decode().strip()
    assert line.startswith("PORT="), "unexpected server output: " + line
    actual_port = int(line[5:])

    # Wait for health.
    deadline = time.time() + 5
    ready = False
    while time.time() < deadline:
        try:
            import urllib.request
            with urllib.request.urlopen("http://127.0.0.1:{}/health".format(actual_port), timeout=1) as r:
                if r.status == 200:
                    ready = True
                    break
        except Exception:
            time.sleep(0.1)
    assert ready, "server did not become healthy"

    yield actual_port

    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()


def test_server_health(running_server):
    """The /health endpoint returns ok."""
    import urllib.request
    port = running_server
    with urllib.request.urlopen("http://127.0.0.1:{}/health".format(port), timeout=2) as r:
        assert r.status == 200
        import json
        data = json.loads(r.read())
        assert data["status"] == "ok"


def test_plugin_send_markdown(running_server, monkeypatch):
    """The plugin's ServerProcess.send_markdown posts to the live server."""
    port = running_server
    # Point the plugin's ServerProcess at the running server.
    sp = fmp.ServerProcess()
    sp._port = port
    # is_running() checks self._process; we simulate a discovered server by
    # setting a dummy process that reports alive.
    class _DummyProc:
        def poll(self):
            return None
    sp._process = _DummyProc()
    assert sp.is_running()
    assert sp.send_markdown("# Hello\n\nWorld")
    assert sp.health_check()


def test_plugin_health_check_dead_port():
    """health_check returns False for a port with no server."""
    sp = fmp.ServerProcess()
    sp._port = _free_port()  # nothing listening
    assert not sp.health_check(timeout=0.5)


def test_sse_delivers_rendered_html(running_server):
    """After a POST /update, the SSE stream delivers rendered HTML."""
    import urllib.request
    port = running_server
    # POST markdown.
    req = urllib.request.Request(
        "http://127.0.0.1:{}/update".format(port),
        data=b"# Title\n\n**bold**",
        method="POST",
        headers={"Content-Type": "text/plain"},
    )
    with urllib.request.urlopen(req, timeout=2) as r:
        assert r.status == 200

    # Connect to SSE via a raw socket so we can read incrementally without
    # http.client's chunked-encoding blocking on a full buffer.
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.settimeout(3)
    sock.connect(("127.0.0.1", port))
    sock.sendall(b"GET /stream HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")

    buf = b""
    deadline = time.time() + 3
    while time.time() < deadline:
        try:
            chunk = sock.recv(8192)
        except socket.timeout:
            break
        if not chunk:
            break
        buf += chunk
        if b"<h1" in buf and b"Title" in buf and b"<strong>bold</strong>" in buf:
            break
    sock.close()
    assert b"event: connected" in buf, "missing connected event"
    assert b"event: update" in buf, "missing update event"
    assert b"<h1" in buf, "missing rendered heading"
    assert b"<strong>bold</strong>" in buf, "missing rendered bold"


def test_index_page_contains_stylesheets(running_server):
    """GET / serves a page with base, syntax-highlight, and dark CSS inlined.

    Regression guard: the browser UI diffs only the #preview node from SSE
    updates, so the stylesheets must live in the index page itself. Without
    them, syntax highlighting and typography render unstyled.
    """
    import urllib.request
    port = running_server
    with urllib.request.urlopen("http://127.0.0.1:{}/".format(port), timeout=2) as r:
        assert r.status == 200
        body = r.read().decode("utf-8")

    assert ".markdown-body" in body, "index missing base markdown styles"
    assert ".chroma" in body, "index missing chroma syntax-highlighting CSS"
    assert ".chroma .k" in body, "index missing chroma keyword token rule"
    assert 'html[data-theme="dark"]' in body, "index missing dark theme overrides"
    # No template placeholders should survive rendering.
    assert "{{ .BaseStyles }}" not in body
    assert "{{ .HighlightCSS }}" not in body
    assert "{{ .DarkStyles }}" not in body
    assert "{{ .Morphdom }}" not in body
