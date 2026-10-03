import { test } from "node:test";
import assert from "node:assert";

test("a nested ESM test in a built directory runs", () => {
  assert.strictEqual(import.meta.url.endsWith("/sub/nested.test.mjs"), true);
});
