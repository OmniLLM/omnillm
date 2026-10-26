import { expect, test } from "bun:test"
import { parseLogLine } from "../../frontend/src/lib/logs"

test("HTTP log stream retains outcome and upload metadata in the console", () => {
  const parsed = parseLogLine("[2026-10-08T12:07:21+08:00] | backend | WARN | Request body read failed | request=upload-test | method=POST | path=/v1/responses | status=400 | latency=1263ms | reason=unexpected_eof | bytes_read=11 | content_length=100")
  expect(parsed.fields).toEqual([
    { key: "request", value: "upload-test" },
    { key: "method", value: "POST" },
    { key: "path", value: "/v1/responses" },
    { key: "status", value: "400" },
    { key: "latency", value: "1263ms" },
    { key: "reason", value: "unexpected_eof" },
    { key: "bytes_read", value: "11" },
    { key: "content_length", value: "100" },
  ])
})
