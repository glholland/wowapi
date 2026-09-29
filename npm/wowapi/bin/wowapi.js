#!/usr/bin/env node
// Launcher for the wowapi Go binary. npm installs exactly one platform package
// (@gholland/wowapi-<platform>-<arch>) through optionalDependencies; this script finds
// its binary and runs it with the same arguments and stdio, so `wowapi mcp`
// works as a stdio MCP server.
"use strict";

const { spawn } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const pkg = `@gholland/wowapi-${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "wowapi.exe" : "wowapi";

function findBinary() {
  if (process.env.WOWAPI_BINARY) return process.env.WOWAPI_BINARY;
  try {
    return path.join(path.dirname(require.resolve(`${pkg}/package.json`)), "bin", exe);
  } catch {
    return null;
  }
}

const bin = findBinary();
if (!bin || !fs.existsSync(bin)) {
  process.stderr.write(
    `wowapi: no prebuilt binary for ${process.platform}-${process.arch} (expected package ${pkg}).\n` +
      "Supported: win32, linux and darwin on x64 and arm64. If your platform is one of these,\n" +
      "reinstall without --no-optional / --omit=optional, or set WOWAPI_BINARY to a wowapi binary.\n",
  );
  process.exit(1);
}

// Some installers drop the executable bit; restore it when we can.
if (process.platform !== "win32") {
  try {
    fs.accessSync(bin, fs.constants.X_OK);
  } catch {
    try {
      fs.chmodSync(bin, 0o755);
    } catch {}
  }
}

const child = spawn(bin, process.argv.slice(2), { stdio: "inherit", windowsHide: true });

// MCP clients stop servers with a signal; pass it on so the binary exits too.
for (const sig of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  process.on(sig, () => {
    if (!child.killed) child.kill(sig);
  });
}

child.on("error", (err) => {
  process.stderr.write(`wowapi: failed to start ${bin}: ${err.message}\n`);
  process.exit(1);
});
child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
  } else {
    process.exit(code ?? 1);
  }
});
