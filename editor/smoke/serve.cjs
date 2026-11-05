// A static file server for the editor's site, for make editor-serve and the
// smoke test. GitHub Pages serves the same files.
//
//   node editor/smoke/serve.cjs build/editor 8000
"use strict";
const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");

const types = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".wasm": "application/wasm",
};

/** Serves root on port (0 picks one); resolves to the server. */
function serve(root, port) {
  root = path.resolve(root);
  const server = http.createServer((req, res) => {
    const url = new URL(req.url, "http://localhost");
    let file = path.join(root, decodeURIComponent(url.pathname));
    if (!file.startsWith(root)) {
      res.writeHead(403).end();
      return;
    }
    if (file.endsWith(path.sep) || url.pathname === "/") file = path.join(file, "index.html");
    fs.readFile(file, (err, data) => {
      if (err) {
        res.writeHead(404).end("not found");
        return;
      }
      res.writeHead(200, { "Content-Type": types[path.extname(file)] || "application/octet-stream" });
      res.end(data);
    });
  });
  return new Promise((resolve) => server.listen(port, "127.0.0.1", () => resolve(server)));
}

module.exports = { serve };

if (require.main === module) {
  const [root = "build/editor", port = "8000"] = process.argv.slice(2);
  serve(root, Number(port)).then((s) => console.log(`serving ${root} on http://localhost:${s.address().port}/`));
}
