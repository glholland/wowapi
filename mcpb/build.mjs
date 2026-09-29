#!/usr/bin/env node
// Builds MCP bundles (.mcpb) for Claude Desktop and other MCPB clients: one
// bundle per platform/CPU, since a manifest can vary its command by OS but not
// by architecture. GoReleaser runs this (see .goreleaser.yaml):
//
//   node mcpb/build.mjs --prepare
//       once, before the builds: records the server's tool list (from
//       `go run . mcp -list -json`, so manifests can't drift from the code)
//       and installs the mcpb CLI.
//
//   node mcpb/build.mjs --version 0.5.0 --binary <path> --target linux_amd64_v1
//       after each build: validates a manifest and packs the bundle into
//       build/mcpb/.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");
const work = path.join(root, "build", "mcpb");
const summaryFile = path.join(work, "summary.json");
const cliDir = path.join(work, "cli");
const MCPB_CLI = "@anthropic-ai/mcpb@2";

const argv = process.argv.slice(2);
const flag = (name) => {
  const i = argv.indexOf(`--${name}`);
  return i >= 0 ? argv[i + 1] : undefined;
};
const fail = (msg) => {
  console.error(`mcpb/build.mjs: ${msg}`);
  process.exit(1);
};

if (argv.includes("--prepare")) {
  fs.rmSync(work, { recursive: true, force: true });
  fs.mkdirSync(work, { recursive: true });
  const summary = execFileSync("go", ["run", ".", "mcp", "-list", "-json"], { cwd: root, encoding: "utf8" });
  fs.writeFileSync(summaryFile, summary);
  // Install the CLI once so the parallel per-target hooks don't race npx.
  execFileSync("npm", ["install", "--no-save", "--no-audit", "--no-fund", "--prefix", cliDir, MCPB_CLI], {
    stdio: "inherit",
    shell: process.platform === "win32",
  });
  console.log(`mcpb: recorded ${JSON.parse(summary).tools.length} tools; CLI ready`);
  process.exit(0);
}

const version = (flag("version") || "").replace(/^v/, "");
const binary = flag("binary");
const target = flag("target") || ""; // GoReleaser target, e.g. darwin_arm64_v8.0
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) fail(`--version must be semver, got ${JSON.stringify(flag("version"))}`);
if (!binary || !fs.existsSync(binary)) fail(`--binary ${JSON.stringify(binary)} does not exist`);
if (!fs.existsSync(summaryFile)) fail("run `node mcpb/build.mjs --prepare` first");

const [goos, goarch] = target.split("_");
const osName = { windows: "win32", darwin: "darwin", linux: "linux" }[goos];
const cpu = { amd64: "x64", arm64: "arm64" }[goarch];
if (!osName || !cpu) fail(`unsupported --target ${JSON.stringify(target)}`);

const summary = JSON.parse(fs.readFileSync(summaryFile, "utf8"));
const exe = osName === "win32" ? "wowapi.exe" : "wowapi";

const longDescription = `Read-only access to Blizzard's official World of Warcraft API.

- **Characters:** profile, gear, stats, professions, reputations, achievements, dungeon and raid progress, Mythic+ seasons
- **Professions:** known vs. missing recipes, and where to get them (boss drops, Auction House prices)
- **New characters:** races, classes, specializations and full talent trees
- **Prices:** region-wide commodity prices on the Auction House

Needs a free Battle.net API client: create one at https://develop.battle.net/access/clients. Character data reflects each character's last logout.`;

const manifest = {
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
  compatibility: { platforms: [osName] },
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
    region: { type: "string", title: "Region", description: "us, eu, kr or tw", default: "us" },
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

const cli = path.join(cliDir, "node_modules", ".bin", process.platform === "win32" ? "mcpb.cmd" : "mcpb");
const run = (...args) => execFileSync(cli, args, { stdio: "inherit", shell: process.platform === "win32" });

const dir = fs.mkdtempSync(path.join(os.tmpdir(), `wowapi-mcpb-${osName}-${cpu}-`));
try {
  fs.mkdirSync(path.join(dir, "server"));
  fs.copyFileSync(binary, path.join(dir, "server", exe));
  fs.chmodSync(path.join(dir, "server", exe), 0o755);
  fs.copyFileSync(path.join(root, "README.md"), path.join(dir, "README.md"));
  fs.writeFileSync(path.join(dir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");

  const out = path.join(work, `wowapi-${version}-${osName}-${cpu}.mcpb`);
  run("validate", path.join(dir, "manifest.json"));
  run("pack", dir, out);
  console.log(`mcpb: built ${path.relative(root, out)}`);
} finally {
  fs.rmSync(dir, { recursive: true, force: true });
}
