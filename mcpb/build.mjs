#!/usr/bin/env node
// Builds MCP bundles (.mcpb) for Claude Desktop and other MCPB clients from
// the cross-compiled binaries in dist/: one bundle per platform/CPU, since a
// manifest can vary its command by OS but not by architecture.
//
// The tool list in each manifest comes from the server itself
// (`wowapi mcp -list -json`), so it can't drift from the code.
//
// Usage: node mcpb/build.mjs --version 0.5.0 [--dist dist] [--out dist]
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");
const MCPB_CLI = "@anthropic-ai/mcpb@2";

const args = Object.fromEntries(
  process.argv.slice(2).reduce((pairs, arg, i, all) => {
    if (arg.startsWith("--")) pairs.push([arg.slice(2), all[i + 1]]);
    return pairs;
  }, []),
);
const version = (args.version || "").replace(/^v/, "");
const distDir = path.resolve(root, args.dist || "dist");
const outDir = path.resolve(root, args.out || "dist");
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error(`mcpb/build.mjs: --version must be semver (e.g. 0.5.0), got ${JSON.stringify(args.version)}`);
  process.exit(1);
}

const targets = [
  { goos: "windows", goarch: "amd64", os: "win32", cpu: "x64" },
  { goos: "windows", goarch: "arm64", os: "win32", cpu: "arm64" },
  { goos: "darwin", goarch: "amd64", os: "darwin", cpu: "x64" },
  { goos: "darwin", goarch: "arm64", os: "darwin", cpu: "arm64" },
  { goos: "linux", goarch: "amd64", os: "linux", cpu: "x64" },
  { goos: "linux", goarch: "arm64", os: "linux", cpu: "arm64" },
];

const distFiles = fs.readdirSync(distDir);
const binaryFor = (t) => {
  const suffix = `-${t.goos}-${t.goarch}${t.goos === "windows" ? ".exe" : ""}`;
  const matches = distFiles.filter((f) => f.startsWith("wowapi-") && f.endsWith(suffix));
  if (matches.length !== 1) {
    console.error(`mcpb/build.mjs: expected one binary ending in ${suffix} in ${distDir}, found ${matches.length}`);
    process.exit(1);
  }
  return path.join(distDir, matches[0]);
};

// Ask the binary for this machine what the server exposes.
const hostCPU = { x64: "amd64", arm64: "arm64" }[process.arch];
const host = targets.find((t) => t.os === process.platform && t.goarch === hostCPU);
if (!host) {
  console.error(`mcpb/build.mjs: no dist binary runs on ${process.platform}-${process.arch}`);
  process.exit(1);
}
const summary = JSON.parse(execFileSync(binaryFor(host), ["mcp", "-list", "-json"], { encoding: "utf8" }));

const longDescription = `Read-only access to Blizzard's official World of Warcraft API.

- **Characters:** profile, gear, stats, professions, reputations, achievements, dungeon and raid progress, Mythic+ seasons
- **Professions:** known vs. missing recipes, and where to get them (boss drops, Auction House prices)
- **New characters:** races, classes, specializations and full talent trees
- **Prices:** region-wide commodity prices on the Auction House

Needs a free Battle.net API client: create one at https://develop.battle.net/access/clients. Character data reflects each character's last logout.`;

function manifest(t) {
  const exe = t.os === "win32" ? "wowapi.exe" : "wowapi";
  return {
    manifest_version: "0.3",
    name: "wowapi",
    display_name: "World of Warcraft (wowapi)",
    version,
    description: "Look up World of Warcraft characters, professions, recipes, talents and Auction House prices from Blizzard's API.",
    long_description: longDescription,
    author: { name: "Garrett Holland", url: "https://github.com/glholland" },
    repository: { type: "git", url: "https://github.com/glholland/wowapi.git" },
    homepage: "https://github.com/glholland/wowapi",
    support: "https://github.com/glholland/wowapi/issues",
    keywords: ["world of warcraft", "wow", "battle.net", "blizzard", "gaming"],
    privacy_policies: ["https://www.blizzard.com/privacy-policy"],
    compatibility: { platforms: [t.os] },
    server: {
      type: "binary",
      entry_point: `server/${exe}`,
      mcp_config: {
        command: `\${__dirname}/server/${exe}`,
        args: ["mcp"],
        env: {
          BLIZZARD_CLIENT_ID: "${user_config.client_id}",
          BLIZZARD_CLIENT_SECRET: "${user_config.client_secret}",
          WOW_REGION: "${user_config.region}",
          WOW_REALM: "${user_config.realm}",
          WOW_CHARACTER: "${user_config.character}",
        },
      },
    },
    tools: summary.tools.map((tool) => ({ name: tool.name, description: tool.description })),
    // Prompts are built by the server from their arguments at request time.
    prompts_generated: true,
    user_config: {
      client_id: {
        type: "string",
        title: "Battle.net client ID",
        description: "From your API client at https://develop.battle.net/access/clients",
        required: true,
      },
      client_secret: {
        type: "string",
        title: "Battle.net client secret",
        description: "The secret for the same API client. Stored securely.",
        sensitive: true,
        required: true,
      },
      region: {
        type: "string",
        title: "Region",
        description: "us, eu, kr or tw",
        default: "us",
      },
      realm: {
        type: "string",
        title: "Default realm",
        description: "Optional. Your main character's realm, e.g. Lightbringer",
        default: "",
      },
      character: {
        type: "string",
        title: "Default character",
        description: "Optional. Your main character's name, exact spelling including accents",
        default: "",
      },
    },
  };
}

fs.mkdirSync(outDir, { recursive: true });
const staging = fs.mkdtempSync(path.join(os.tmpdir(), "wowapi-mcpb-"));
const npx = process.platform === "win32" ? "npx.cmd" : "npx";
try {
  for (const t of targets) {
    const dir = path.join(staging, `${t.os}-${t.cpu}`);
    const exe = t.os === "win32" ? "wowapi.exe" : "wowapi";
    fs.mkdirSync(path.join(dir, "server"), { recursive: true });
    fs.copyFileSync(binaryFor(t), path.join(dir, "server", exe));
    fs.chmodSync(path.join(dir, "server", exe), 0o755);
    fs.copyFileSync(path.join(root, "README.md"), path.join(dir, "README.md"));
    fs.writeFileSync(path.join(dir, "manifest.json"), JSON.stringify(manifest(t), null, 2) + "\n");

    const out = path.join(outDir, `wowapi-${version}-${t.os}-${t.cpu}.mcpb`);
    execFileSync(npx, ["-y", MCPB_CLI, "validate", path.join(dir, "manifest.json")], { stdio: "inherit", shell: process.platform === "win32" });
    execFileSync(npx, ["-y", MCPB_CLI, "pack", dir, out], { stdio: "inherit", shell: process.platform === "win32" });
    console.log(`built ${path.relative(root, out)}`);
  }
} finally {
  fs.rmSync(staging, { recursive: true, force: true });
}
