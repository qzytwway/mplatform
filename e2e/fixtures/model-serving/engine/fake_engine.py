"""A stand-in for vLLM and SGLang servers in model serving e2e tests.

It accepts the command lines the platform renders, prints them with the
environment so tests can assert on them, and serves the HTTP endpoints that
probes, metrics scrapers and the SGLang router call. No GPU or model weights
are needed.

Log lines start with a tag and carry one JSON object:
  FAKE_ENGINE_START     argv, env and what was parsed from them
  FAKE_ENGINE_ERROR     why the engine refused to start
  FAKE_ENGINE_DIST      how this node reached the leader of its group
  FAKE_ENGINE_REQUEST   every HTTP request served
  FAKE_ENGINE_BOOTSTRAP a decode server reaching the prefill bootstrap server
  FAKE_ENGINE_KV        a vLLM decode server reaching the prefill side channel
"""

import json
import os
import signal
import socket
import sys
import threading
import time
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LEADER_WAIT_SECONDS = 300
RETRY_SECONDS = 2
FAILURE_REPORT_ATTEMPTS = 5
CONNECT_TIMEOUT_SECONDS = 5
# vLLM's NixlConnector serves its handshake here unless told otherwise.
NIXL_SIDE_CHANNEL_PORT = 5600
LATENCY_BUCKETS = ("0.1", "0.5", "1.0", "5.0", "+Inf")


def log(tag, payload):
    print(tag, json.dumps(payload, sort_keys=True), flush=True)


def option(argv, name):
    """Returns the value of the last occurrence of --name, as argparse does."""
    value = None
    for i, arg in enumerate(argv):
        if arg == name and i + 1 < len(argv):
            value = argv[i + 1]
        elif arg.startswith(name + "="):
            value = arg.split("=", 1)[1]
    return value


def parse(runtime, argv):
    model_path = option(argv, "--model-path") if runtime == "sglang" else (argv[0] if argv else None)
    port = option(argv, "--port")
    kv_config = option(argv, "--kv-transfer-config")
    return {
        "runtime": runtime,
        "modelPath": model_path,
        "modelPathExists": bool(model_path) and os.path.isdir(model_path),
        "servedModelName": option(argv, "--served-model-name") or model_path,
        "host": option(argv, "--host"),
        "port": int(port) if port else None,
        "tp": int(option(argv, "--tp") or option(argv, "--tensor-parallel-size") or 1),
        "nnodes": int(option(argv, "--nnodes") or 1),
        "nodeRank": int(option(argv, "--node-rank") or 0),
        "distInitAddr": option(argv, "--dist-init-addr"),
        "masterAddr": option(argv, "--master-addr"),
        "disaggregationMode": option(argv, "--disaggregation-mode"),
        "bootstrapPort": int(option(argv, "--disaggregation-bootstrap-port") or 0) or None,
        "transferBackend": option(argv, "--disaggregation-transfer-backend"),
        "kvTransferConfig": json.loads(kv_config) if kv_config else None,
    }


def first(value):
    return value[0] if isinstance(value, list) else value


class Engine:
    def __init__(self, runtime, argv):
        self.argv = argv
        self.info = parse(runtime, argv)
        # A multi-node SGLang leader serves once every node joined, as the
        # real engine only loads the model when the group is complete.
        self.peers_needed = self.info["nnodes"] - 1 if self.leads_dist_init() else 0
        self.peers = 0
        self.requests = 0
        self.lock = threading.Lock()

    def leads_dist_init(self):
        return self.info["distInitAddr"] is not None and self.info["nodeRank"] == 0

    def ready(self):
        with self.lock:
            return self.peers >= self.peers_needed

    def peer_joined(self):
        with self.lock:
            self.peers += 1

    def request_served(self):
        with self.lock:
            self.requests += 1

    def server_info(self):
        info = self.info
        return {
            "model_path": info["modelPath"],
            "served_model_name": info["servedModelName"],
            "tp_size": info["tp"],
            "dp_size": 1,
            "nnodes": info["nnodes"],
            "node_rank": info["nodeRank"],
            "disaggregation_mode": info["disaggregationMode"] or "null",
            "disaggregation_bootstrap_port": info["bootstrapPort"],
            "max_total_num_tokens": 65536,
            "launch_command": " ".join(self.argv),
            "version": "0.0.0-fake",
        }

    def model_info(self):
        info = self.info
        return {
            "model_path": info["modelPath"],
            "served_model_name": info["servedModelName"],
            "tokenizer_path": info["modelPath"],
            "is_generation": True,
            "model_type": "llama",
            "architectures": ["LlamaForCausalLM"],
            "disaggregation_mode": info["disaggregationMode"] or "null",
        }

    def metrics(self):
        """Exposes the metric names the real engines do, with fake values."""
        sglang = self.info["runtime"] == "sglang"
        prefix = "sglang:" if sglang else "vllm:"
        label = f'model_name="{self.info["servedModelName"]}"'
        with self.lock:
            requests = self.requests
        gauges = (
            ["num_running_reqs", "num_queue_reqs", "token_usage", "gen_throughput", "cache_hit_rate"]
            if sglang
            else ["num_requests_running", "num_requests_waiting", "gpu_cache_usage_perc", "kv_cache_usage_perc"]
        )
        counters = {
            "prompt_tokens_total": requests,
            "generation_tokens_total": 3 * requests,
            ("num_requests_total" if sglang else "request_success_total"): requests,
        }
        histograms = ["time_to_first_token_seconds", "e2e_request_latency_seconds"] + (
            ["inter_token_latency_seconds"] if sglang else ["time_per_output_token_seconds"]
        )
        lines = []
        for name in gauges:
            lines += [f"# TYPE {prefix}{name} gauge", f"{prefix}{name}{{{label}}} 0"]
        for name, value in counters.items():
            lines += [f"# TYPE {prefix}{name} counter", f"{prefix}{name}{{{label}}} {value}"]
        for name in histograms:
            lines.append(f"# TYPE {prefix}{name} histogram")
            lines += [f'{prefix}{name}_bucket{{{label},le="{le}"}} {requests}' for le in LATENCY_BUCKETS]
            lines += [f"{prefix}{name}_sum{{{label}}} {0.05 * requests}", f"{prefix}{name}_count{{{label}}} {requests}"]
        return "\n".join(lines) + "\n"

    def completion(self, path, body, extra=None):
        name = self.info["servedModelName"]
        text = f"fake {self.info['disaggregationMode'] or 'engine'} reply"
        if path == "/generate":
            return {"text": text, "meta_info": {"id": "fake", "finish_reason": {"type": "stop"}}}
        if path == "/v1/chat/completions":
            choice = {"index": 0, "message": {"role": "assistant", "content": text}, "finish_reason": "stop"}
            kind = "chat.completion"
        else:
            choice = {"index": 0, "text": text, "finish_reason": "stop"}
            kind = "text_completion"
        reply = {
            "id": "fake",
            "object": kind,
            "created": int(time.time()),
            "model": body.get("model", name),
            "choices": [choice],
            "usage": {"prompt_tokens": 1, "completion_tokens": 3, "total_tokens": 4},
        }
        reply.update(extra or {})
        return reply

    def reach_bootstrap(self, body):
        """Makes an SGLang decode server fetch the prefill server's bootstrap
        info from the address the router injected, as the real one does."""
        host, port, room = (first(body.get(key)) for key in ("bootstrap_host", "bootstrap_port", "bootstrap_room"))
        # Rooms are 64-bit; a string keeps them exact for JavaScript readers.
        result = {"host": host, "port": port, "room": None if room is None else str(room), "status": None, "error": None}
        if host is None or port is None:
            result["error"] = "the request carries no bootstrap_host and bootstrap_port"
        else:
            query = urllib.parse.urlencode({"bootstrap_room": room})
            try:
                with urllib.request.urlopen(f"http://{host}:{port}/route?{query}", timeout=CONNECT_TIMEOUT_SECONDS) as response:
                    result["status"] = response.status
            except OSError as err:
                result["error"] = str(err)
        log("FAKE_ENGINE_BOOTSTRAP", result)
        return result["status"] == 200

    def side_channel_host(self):
        return os.environ.get("VLLM_NIXL_SIDE_CHANNEL_HOST") or socket.gethostbyname(socket.gethostname())

    def side_channel_port(self):
        return int(os.environ.get("VLLM_NIXL_SIDE_CHANNEL_PORT") or NIXL_SIDE_CHANNEL_PORT)

    def kv_transfer(self, params):
        """Plays vLLM's NixlConnector: a prefill asked to hand off the KV
        cache returns where to fetch it, and a decode given those fields
        connects there. Returns the reply fields and whether it succeeded."""
        if params.get("do_remote_decode"):
            return {
                "kv_transfer_params": {
                    "do_remote_prefill": True,
                    "do_remote_decode": False,
                    "remote_engine_id": "fake-engine",
                    "remote_block_ids": [0],
                    "remote_host": self.side_channel_host(),
                    "remote_port": self.side_channel_port(),
                }
            }, True
        if not params.get("do_remote_prefill"):
            return {}, True
        result = {"host": params.get("remote_host"), "port": params.get("remote_port"), "connected": False, "error": None}
        try:
            socket.create_connection((result["host"], int(result["port"])), timeout=CONNECT_TIMEOUT_SECONDS).close()
            result["connected"] = True
        except (OSError, TypeError, ValueError) as err:
            result["error"] = str(err)
        log("FAKE_ENGINE_KV", result)
        return {}, result["connected"]


def handler_for(engine, server_name):
    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, format, *args):
            pass

        def reply(self, status, payload, content_type="application/json"):
            body = payload.encode() if isinstance(payload, str) else json.dumps(payload).encode()
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def record(self, body=None):
            log("FAKE_ENGINE_REQUEST", {"server": server_name, "method": self.command, "path": self.path, "body": body})

        def do_GET(self):
            self.record()
            path = self.path.split("?", 1)[0]
            if server_name == "bootstrap":
                self.reply(200, {"status": "ok"})
            elif path in ("/health", "/health_generate", "/ping"):
                if engine.ready():
                    self.reply(200, {"status": "ok"})
                else:
                    self.reply(503, {"status": "waiting for nodes"})
            elif path in ("/server_info", "/get_server_info"):
                self.reply(200, engine.server_info())
            elif path in ("/model_info", "/get_model_info"):
                self.reply(200, engine.model_info())
            elif path == "/v1/models":
                name = engine.info["servedModelName"]
                self.reply(200, {"object": "list", "data": [{"id": name, "object": "model", "owned_by": "fake"}]})
            elif path == "/metrics":
                self.reply(200, engine.metrics(), "text/plain; version=0.0.4")
            elif path == "/get_load":
                self.reply(200, {"load": 0})
            else:
                self.reply(404, {"error": f"fake engine does not serve {path}"})

        def do_POST(self):
            length = int(self.headers.get("Content-Length") or 0)
            raw = self.rfile.read(length) if length else b""
            try:
                body = json.loads(raw) if raw else {}
            except ValueError:
                body = {"raw": raw.decode(errors="replace")}
            self.record(body)
            path = self.path.split("?", 1)[0]
            if path not in ("/generate", "/v1/completions", "/v1/chat/completions"):
                if path == "/flush_cache":
                    self.reply(200, {"status": "ok"})
                else:
                    self.reply(404, {"error": f"fake engine does not serve {path}"})
                return
            engine.request_served()
            if engine.info["disaggregationMode"] == "decode" and not engine.reach_bootstrap(body):
                self.reply(502, {"error": "decode could not reach the prefill bootstrap server"})
                return
            extra, ok = engine.kv_transfer(body.get("kv_transfer_params") or {})
            if not ok:
                self.reply(502, {"error": "decode could not reach the prefill KV side channel"})
                return
            self.reply(200, engine.completion(path, body, extra))

    return Handler


def serve(engine, port, server_name):
    server = ThreadingHTTPServer(("0.0.0.0", port), handler_for(engine, server_name))
    threading.Thread(target=server.serve_forever, daemon=True).start()


def listen(port, on_connect):
    listener = socket.create_server(("0.0.0.0", port))

    def loop():
        while True:
            conn, address = listener.accept()
            on_connect(address[0])
            conn.close()

    threading.Thread(target=loop, daemon=True).start()


def join_leader(engine):
    """Resolves the group leader and, for SGLang, connects to its dist-init port."""
    info = engine.info
    if info["distInitAddr"]:
        host, _, port = info["distInitAddr"].rpartition(":")
    else:
        host, port = info["masterAddr"], ""
    deadline = time.time() + LEADER_WAIT_SECONDS
    attempt = 0
    while time.time() < deadline:
        result = {"role": "worker", "leader": host, "resolved": None, "connected": False, "error": None}
        try:
            result["resolved"] = socket.gethostbyname(host)
            if port:
                socket.create_connection((host, int(port)), timeout=CONNECT_TIMEOUT_SECONDS).close()
                result["connected"] = True
            log("FAKE_ENGINE_DIST", result)
            return
        except OSError as err:
            result["error"] = str(err)
        # Report failures now and then, so tests need not wait out the deadline.
        if attempt % FAILURE_REPORT_ATTEMPTS == 0:
            log("FAKE_ENGINE_DIST", result)
        attempt += 1
        time.sleep(RETRY_SECONDS)


def main(runtime, argv):
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    engine = Engine(runtime, argv)
    log("FAKE_ENGINE_START", {"argv": argv, "env": dict(os.environ), "parsed": engine.info})
    # The kubelet expands $(VAR) only for variables in the container env; a
    # real engine would fail on the literal, so this one does too.
    unexpanded = [arg for arg in argv if "$(LWS_" in arg]
    if unexpanded:
        log("FAKE_ENGINE_ERROR", {"unexpanded": unexpanded})
        sys.exit(1)

    info = engine.info
    if engine.leads_dist_init():
        port = int(info["distInitAddr"].rpartition(":")[2])

        def peer_joined(peer):
            engine.peer_joined()
            log("FAKE_ENGINE_DIST", {"role": "leader", "peer": peer})

        listen(port, peer_joined)
    elif info["nodeRank"] > 0 and (info["distInitAddr"] or info["masterAddr"]):
        threading.Thread(target=join_leader, args=(engine,), daemon=True).start()
    if info["port"]:
        serve(engine, info["port"], "http")
    # Every node of a prefill group runs with the bootstrap port, but only the
    # leader hosts the bootstrap server decode servers reach.
    if info["disaggregationMode"] == "prefill" and info["bootstrapPort"] and info["nodeRank"] == 0:
        serve(engine, info["bootstrapPort"], "bootstrap")
    if info["kvTransferConfig"] and info["port"]:
        listen(engine.side_channel_port(), lambda peer: log("FAKE_ENGINE_KV", {"role": "prefill", "peer": peer}))
    while True:
        time.sleep(3600)


if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] != "serve":
        sys.exit("usage: vllm serve <model> [options]")
    main("vllm", sys.argv[2:])
