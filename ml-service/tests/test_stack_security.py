"""OPS-02: the local stack fails closed, and its defaults do not hand it away.

Three things were true of `docker compose up` at once, and each one undid a protection the
code already had:

* `API_KEY: ${API_KEY:-dev-local-key}` gave every checkout of this repository the same
  admin key. `go-app/internal/server/auth.go` deliberately fails closed when `API_KEY` is
  unset -- it answers 503 rather than let an unauthenticated request through -- and that
  fallback was the only reason it never did. The ML service is worse off still: its
  `_verify_admin_api_key` returns early when `ADMIN_API_KEY` is empty, so a stack started
  without a key would have an *open* admin surface rather than a closed one.
* Postgres published `5432:5432` and both APIs published on every interface, with
  `POSTGRES_PASSWORD` defaulting to `postgres`, so any host on the same network could read
  and write the archive.
* The watcher mounted `/var/run/docker.sock`, which is root on the host.

These tests read the committed compose file and the targets that talk to the stack, so a
default key or an unbound port cannot come back unnoticed. They parse text rather than
YAML on purpose: there is no YAML parser in this service's dependencies, and adding one to
assert a property of a file nothing imports would be the wrong trade.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import Dict, List

REPO_ROOT = Path(__file__).resolve().parents[2]
COMPOSE_FILE = REPO_ROOT / "docker-compose.yml"
ROOT_MAKEFILE = REPO_ROOT / "Makefile"
ML_SERVICE_MAKEFILE = REPO_ROOT / "ml-service" / "Makefile"
CADENCE_SCRIPT = REPO_ROOT / "scripts" / "cadence.sh"

# The compose services that publish a port to the host, and the port each one publishes.
PUBLISHED_PORTS = {"postgres": "5432", "go-api": "8080", "ml-service": "8000"}

# The key the stack used to fall back to. It is a shared secret in the literal sense: every
# clone of this repository knows it.
RETIRED_DEFAULT_KEY = "dev-local-key"

SERVICE_HEADER = re.compile(r"^  ([A-Za-z][A-Za-z0-9_-]*):$")
PORT_MAPPING = re.compile(r'^\s*-\s*"([^"]+)"\s*$')


def compose_services() -> Dict[str, str]:
    """The compose file split into one block of text per top-level service, comments
    dropped -- a comment explaining why a service does not mount the Docker socket must not
    read as the service mounting it."""
    services: Dict[str, str] = {}
    current: str | None = None
    lines: List[str] = []
    for line in COMPOSE_FILE.read_text().splitlines():
        if line.lstrip().startswith("#"):
            continue
        header = SERVICE_HEADER.match(line)
        if header:
            if current is not None:
                services[current] = "\n".join(lines)
            current, lines = header.group(1), []
            continue
        if line and not line.startswith(" ") and not line.startswith("#"):
            # A top-level key such as `volumes:` ends the last service's block.
            if current is not None:
                services[current] = "\n".join(lines)
            current, lines = None, []
            continue
        if current is not None:
            lines.append(line)
    if current is not None:
        services[current] = "\n".join(lines)
    return services


def test_the_compose_stack_has_no_default_admin_key() -> None:
    """`API_KEY` and `ADMIN_API_KEY` carry no fallback value: compose must refuse to render
    without one rather than start the stack with a key everyone knows."""
    compose = COMPOSE_FILE.read_text()

    assert RETIRED_DEFAULT_KEY not in compose
    assert re.search(r"API_KEY:\s*\$\{API_KEY:\?", compose), "API_KEY must use the required `:?` form"
    assert not re.search(r"API_KEY:\s*\$\{[A-Z_]*API_KEY:-[^$]", compose), "no `:-` default for any API key"


def test_the_ml_services_admin_key_is_never_empty() -> None:
    """`_verify_admin_api_key` lets every admin call through when `ADMIN_API_KEY` is empty,
    so the compose stack must always supply one -- here, by falling back to the API_KEY it
    already refuses to render without."""
    ml_service = compose_services()["ml-service"]

    assert re.search(r"ADMIN_API_KEY:\s*\$\{ADMIN_API_KEY:-\$\{API_KEY:\?", ml_service)


def test_every_published_port_binds_to_an_explicit_host() -> None:
    """A bare `5432:5432` binds to every interface. Each published port names the host it
    binds to, defaulting to loopback, so the archive and both APIs are reachable from this
    machine only unless a developer says otherwise."""
    services = compose_services()

    for name, port in PUBLISHED_PORTS.items():
        mappings = [
            PORT_MAPPING.match(line).group(1) for line in services[name].splitlines() if PORT_MAPPING.match(line)
        ]
        published = [mapping for mapping in mappings if mapping.endswith(f":{port}")]
        assert published, f"{name} publishes no {port} mapping"
        for mapping in published:
            # Rightmost first: the host part may itself hold a colon, as `${BIND_HOST:-...}` does.
            parts = mapping.rsplit(":", 2)
            assert len(parts) == 3, f"{name}: {mapping} names no host to bind to"
            host, host_port, container_port = parts
            assert (host_port, container_port) == (port, port), f"{name}: {mapping} is not <host>:{port}:{port}"
            assert host in ("127.0.0.1", "${BIND_HOST:-127.0.0.1}"), f"{name} binds {port} to {host!r}"


def test_the_watcher_does_not_hold_the_host_docker_socket() -> None:
    """Mounting the socket into the watcher is root on the host. It reads only -- events,
    inspect and logs are GET calls -- so it goes through the read-only socket proxy."""
    services = compose_services()

    assert "/var/run/docker.sock" not in services["watcher"]
    assert "DOCKER_HOST: tcp://docker-socket-proxy:2375" in services["watcher"]
    assert "/var/run/docker.sock:/var/run/docker.sock:ro" in services["docker-socket-proxy"]
    assert "POST: 0" in services["docker-socket-proxy"], "the proxy must refuse every write call"


def test_the_socket_is_mounted_exactly_once_in_the_whole_stack() -> None:
    """Only the proxy may see it; a second service mounting it would undo the point."""
    holders = [name for name, block in compose_services().items() if "/var/run/docker.sock" in block]

    assert holders == ["docker-socket-proxy"]


def test_no_make_target_or_script_falls_back_to_the_retired_key() -> None:
    """`make reload`, `make cadence` and scripts/cadence.sh each used to default to it, so
    a stack started with a real key was still driven with the shared one by anything that
    forgot to pass it."""
    for path in (ROOT_MAKEFILE, ML_SERVICE_MAKEFILE, CADENCE_SCRIPT):
        assert RETIRED_DEFAULT_KEY not in path.read_text(), f"{path} still defaults to the retired key"


def test_the_targets_that_need_the_key_refuse_without_one() -> None:
    """Failing closed is only useful if it says what to set: the root Makefile gates the
    two targets that talk to a running stack on `require-api-key`, and cadence.sh treats a
    missing key as a precondition failure rather than sending an empty header."""
    root_makefile = ROOT_MAKEFILE.read_text()

    assert re.search(r"^reload: require-api-key$", root_makefile, re.MULTILINE)
    assert re.search(r"^cadence: require-api-key$", root_makefile, re.MULTILINE)
    assert 'test -n "$(API_KEY)"' in ML_SERVICE_MAKEFILE.read_text()
    assert '[ -n "$API_KEY" ] ||' in CADENCE_SCRIPT.read_text()
