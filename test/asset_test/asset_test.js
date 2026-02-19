const { strict: assert } = require("node:assert");

// Image imports — each should resolve to a string path with the right extension.
const png = require("./fixtures/image.png");
const svg = require("./fixtures/image.svg");
const jpg = require("./fixtures/image.jpg");
const gif = require("./fixtures/image.gif");
const webp = require("./fixtures/image.webp");

// Font imports
const woff2 = require("./fixtures/font.woff2");
const ttf = require("./fixtures/font.ttf");

// Media imports
const mp3 = require("./fixtures/audio.mp3");

// 1. Image imports return strings ending in the correct extension
assert.equal(typeof png, "string", "PNG import should be a string");
assert.ok(png.endsWith(".png"), `expected .png, got: ${png}`);

assert.equal(typeof svg, "string", "SVG import should be a string");
assert.ok(svg.endsWith(".svg"), `expected .svg, got: ${svg}`);

assert.equal(typeof jpg, "string", "JPG import should be a string");
assert.ok(jpg.endsWith(".jpg"), `expected .jpg, got: ${jpg}`);

assert.equal(typeof gif, "string", "GIF import should be a string");
assert.ok(gif.endsWith(".gif"), `expected .gif, got: ${gif}`);

assert.equal(typeof webp, "string", "WebP import should be a string");
assert.ok(webp.endsWith(".webp"), `expected .webp, got: ${webp}`);

// 2. Font imports return strings
assert.equal(typeof woff2, "string", "WOFF2 import should be a string");
assert.ok(woff2.endsWith(".woff2"), `expected .woff2, got: ${woff2}`);

assert.equal(typeof ttf, "string", "TTF import should be a string");
assert.ok(ttf.endsWith(".ttf"), `expected .ttf, got: ${ttf}`);

// 3. Media imports return strings
assert.equal(typeof mp3, "string", "MP3 import should be a string");
assert.ok(mp3.endsWith(".mp3"), `expected .mp3, got: ${mp3}`);

// 4. All imports are distinct (no collisions)
const all = [png, svg, jpg, gif, webp, woff2, ttf, mp3];
const unique = new Set(all);
assert.equal(unique.size, all.length, "all asset paths should be unique");

console.log("asset_test: all assertions passed");
