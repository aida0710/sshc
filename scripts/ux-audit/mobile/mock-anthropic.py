"""スマホの点検（2026-11-03）で Claude Code の画面を動かすための、Messages API の作りものの応答。

本物のモデルには送らない。最初の依頼には Bash（ls -la）の道具の呼び出しを返して許可の確認を出させ、
道具の結果が来たら日本語の長い文（差分と箇条書き）を返す。ストリーミングと非ストリーミングの両方に答える。
"""
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ANSWER = "\n".join(
    ["ファイルの一覧を確かめました。次のように直します。", "", "```diff", "- func main() {}", "+ func main() {", '+\tfmt.Println("こんにちは")', "+ }", "```", ""]
    + [f"- 手順{n}: README.md と src/main.go と docs/design.md を読み、変更点を確かめます。" for n in range(1, 31)]
)


def events(blocks, stop_reason):
    yield "message_start", {"type": "message_start", "message": {"id": "msg_mock", "type": "message", "role": "assistant", "model": "claude-mock", "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 10, "output_tokens": 1}}}
    for index, block in enumerate(blocks):
        if block["type"] == "text":
            yield "content_block_start", {"type": "content_block_start", "index": index, "content_block": {"type": "text", "text": ""}}
            for start in range(0, len(block["text"]), 40):
                yield "content_block_delta", {"type": "content_block_delta", "index": index, "delta": {"type": "text_delta", "text": block["text"][start:start + 40]}}
        else:
            yield "content_block_start", {"type": "content_block_start", "index": index, "content_block": {"type": "tool_use", "id": block["id"], "name": block["name"], "input": {}}}
            yield "content_block_delta", {"type": "content_block_delta", "index": index, "delta": {"type": "input_json_delta", "partial_json": json.dumps(block["input"])}}
        yield "content_block_stop", {"type": "content_block_stop", "index": index}
    yield "message_delta", {"type": "message_delta", "delta": {"stop_reason": stop_reason, "stop_sequence": None}, "usage": {"output_tokens": 20}}
    yield "message_stop", {"type": "message_stop"}


def text_of(message):
    content = message.get("content")
    if isinstance(content, str):
        return content
    return " ".join(part.get("text", "") for part in content or [] if isinstance(part, dict))


def reply(request):
    """依頼の文に「一覧」があり、まだ道具を呼んでいない会話にだけ Bash の呼び出しを返す。
    ほかの呼び出し（題名づけ、続きの提案など）と、道具の結果のあとは文で答える。"""
    messages = request.get("messages", [])
    called = isinstance(messages[-1].get("content") if messages else None, list) and any(isinstance(p, dict) and p.get("type") == "tool_result" for p in messages[-1]["content"])
    last_user = next((text_of(m) for m in reversed(messages) if m.get("role") == "user"), "")
    if request.get("tools") and not called and "一覧" in last_user:
        return [{"type": "text", "text": "ファイルを確かめます。"}, {"type": "tool_use", "id": "toolu_mock_1", "name": "Bash", "input": {"command": "date > note.txt && ls -la", "description": "日時を note.txt に書いてから一覧する"}}], "tool_use"
    return [{"type": "text", "text": ANSWER if called else "mock"}], "end_turn"


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        if self.path.startswith("/v1/messages/count_tokens"):
            return self.json({"input_tokens": 10})
        if not self.path.startswith("/v1/messages"):
            return self.json({})
        blocks, stop = reply(body)
        # /tmp/mock-delay に秒数を書くと、道具の呼び出しを返す前に待つ（アプリを裏に回してから許可の確認が来る場面を作るため）
        try:
            if stop == "tool_use":
                time.sleep(float(open("/tmp/mock-delay").read()))
        except (OSError, ValueError):
            pass
        if body.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            for name, data in events(blocks, stop):
                self.wfile.write(f"event: {name}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n".encode())
                self.wfile.flush()
            return
        self.json({"id": "msg_mock", "type": "message", "role": "assistant", "model": "claude-mock", "content": blocks, "stop_reason": stop, "stop_sequence": None, "usage": {"input_tokens": 10, "output_tokens": 20}})

    def do_GET(self):
        self.json({})

    def json(self, value):
        data = json.dumps(value, ensure_ascii=False).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


ThreadingHTTPServer(("127.0.0.1", 8099), Handler).serve_forever()
