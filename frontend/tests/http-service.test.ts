import { test } from "node:test";
import assert from "node:assert/strict";
import { HttpManagerService } from "../src/lib/http-service";

test("static site uploads send a valid multipart body and reload the published metadata", async () => {
  const originalFetch = globalThis.fetch;
  const service = new HttpManagerService();
  const snapshot = service.getSnapshot();
  const calls: string[] = [];
  globalThis.fetch = async (input, init) => {
    const request = new Request("https://manager.example" + input, init);
    calls.push(request.method + " " + new URL(request.url).pathname);
    if (request.url.endsWith("/instances/test-instance/site")) {
      assert.match(
        request.headers.get("Content-Type") ?? "",
        /^multipart\/form-data; boundary=/,
      );
      const form = await request.formData();
      const file = form.get("site") as File;
      assert.equal(file.name, "index.html");
      assert.equal(await file.text(), "<h1>Website</h1>");
      return Response.json({
        filename: file.name,
        files: 1,
        bytes: file.size,
        publishedAt: new Date().toISOString(),
      });
    }
    assert.ok(request.url.endsWith("/refresh"));
    return Response.json(snapshot);
  };
  try {
    await service.uploadSite(
      "test-instance",
      new File(["<h1>Website</h1>"], "index.html", { type: "text/html" }),
    );
    assert.deepEqual(calls, [
      "POST /api/manager/instances/test-instance/site",
      "POST /api/manager/refresh",
    ]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});
