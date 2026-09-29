#!/usr/bin/env node
// Builds the npm packages from the cross-compiled binaries in dist/:
//
//   wowapi                 launcher (bin/wowapi.js) + optionalDependencies
//   wowapi-<os>-<cpu>      one per platform, holding only that binary
//
// npm installs just the platform package matching the user's machine (via the
// os/cpu fields), the same pattern esbuild and Biome use.
//
// Binaries come from GoReleaser's dist/artifacts.json, or with --assets from a
// folder of GitHub release downloads (wowapi-<tag>-<goos>-<goarch>[.exe]); the
// latter is how the first npm version was bootstrapped (task npm:bootstrap).
// Usage: node npm/build.mjs --version 0.5.0 [--dist dist | --assets dir] [--out npm/dist]
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");

const args = Object.fromEntries(
  process.argv.slice(2).reduce((pairs, arg, i, all) => {
    if (arg.startsWith("--")) pairs.push([arg.slice(2), all[i + 1]]);
    return pairs;
  }, []),
);
const version = (args.version || "").replace(/^v/, "");
const distDir = path.resolve(root, args.dist || "dist");
const outDir = path.resolve(root, args.out || "npm/dist");
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error(`npm/build.mjs: --version must be semver (e.g. 0.5.0), got ${JSON.stringify(args.version)}`);
  process.exit(1);
}

// Go GOOS/GOARCH -> Node process.platform/process.arch.
const targets = [
  { goos: "windows", goarch: "amd64", os: "win32", cpu: "x64" },
  { goos: "windows", goarch: "arm64", os: "win32", cpu: "arm64" },
  { goos: "linux", goarch: "amd64", os: "linux", cpu: "x64" },
  { goos: "linux", goarch: "arm64", os: "linux", cpu: "arm64" },
  { goos: "darwin", goarch: "amd64", os: "darwin", cpu: "x64" },
  { goos: "darwin", goarch: "arm64", os: "darwin", cpu: "arm64" },
];

const common = {
  version,
  license: "MIT",
  homepage: "https://github.com/glholland/wowapi#readme",
  repository: { type: "git", url: "git+https://github.com/glholland/wowapi.git" },
};

const write = (dir, file, data) => {
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, file), typeof data === "string" ? data : JSON.stringify(data, null, 2) + "\n");
};

fs.rmSync(outDir, { recursive: true, force: true });

// GoReleaser records every binary it built in dist/artifacts.json.
const fail = (msg) => {
  console.error(`npm/build.mjs: ${msg}`);
  process.exit(1);
};

let binaryFor;
if (args.assets) {
  const dir = path.resolve(root, args.assets);
  const files = fs.readdirSync(dir);
  binaryFor = (t) => {
    const suffix = `-${t.goos}-${t.goarch}${t.goos === "windows" ? ".exe" : ""}`;
    const matches = files.filter((f) => f.startsWith("wowapi-") && f.endsWith(suffix));
    if (matches.length !== 1) fail(`expected one release asset ending in ${suffix} in ${dir}, found ${matches.length}`);
    return path.join(dir, matches[0]);
  };
} else {
  const artifactsFile = path.join(distDir, "artifacts.json");
  if (!fs.existsSync(artifactsFile)) fail(`${artifactsFile} not found; run GoReleaser first (task release)`);
  const artifacts = JSON.parse(fs.readFileSync(artifactsFile, "utf8"));
  binaryFor = (t) => {
    const found = artifacts.find((a) => a.type === "Binary" && a.goos === t.goos && a.goarch === t.goarch);
    if (!found) fail(`no ${t.goos}/${t.goarch} binary in ${artifactsFile}`);
    return path.resolve(root, found.path);
  };
}
const optionalDependencies = {};

for (const t of targets) {
  const name = `wowapi-${t.os}-${t.cpu}`;
  const dir = path.join(outDir, name);
  const exe = t.os === "win32" ? "wowapi.exe" : "wowapi";
  fs.mkdirSync(path.join(dir, "bin"), { recursive: true });
  fs.copyFileSync(binaryFor(t), path.join(dir, "bin", exe));
  fs.chmodSync(path.join(dir, "bin", exe), 0o755);
  write(dir, "package.json", {
    name,
    ...common,
    description: `The ${t.os}-${t.cpu} binary for wowapi. Install "wowapi" instead of this package.`,
    os: [t.os],
    cpu: [t.cpu],
    files: ["bin", "LICENSE"],
    preferUnplugged: true,
  });
  fs.copyFileSync(path.join(root, "LICENSE"), path.join(dir, "LICENSE"));
  write(dir, "README.md", `# ${name}\n\nThe ${t.os}-${t.cpu} binary for [wowapi](https://www.npmjs.com/package/wowapi). Install \`wowapi\` instead.\n`);
  optionalDependencies[name] = version;
}

const main = path.join(outDir, "wowapi");
fs.mkdirSync(path.join(main, "bin"), { recursive: true });
fs.copyFileSync(path.join(here, "wowapi", "bin", "wowapi.js"), path.join(main, "bin", "wowapi.js"));
fs.chmodSync(path.join(main, "bin", "wowapi.js"), 0o755);
fs.copyFileSync(path.join(root, "README.md"), path.join(main, "README.md"));
fs.copyFileSync(path.join(root, "LICENSE"), path.join(main, "LICENSE"));
write(main, "package.json", {
  name: "wowapi",
  ...common,
  description: "World of Warcraft Battle.net API client and MCP server: characters, professions, recipes, talents and Auction House prices.",
  keywords: ["mcp", "mcp-server", "model-context-protocol", "world-of-warcraft", "wow", "battle.net", "blizzard"],
  bin: { wowapi: "bin/wowapi.js" },
  files: ["bin", "README.md", "LICENSE"],
  engines: { node: ">=18" },
  optionalDependencies,
});

console.log(`Built ${targets.length + 1} npm packages for ${version} in ${path.relative(root, outDir)}`);
