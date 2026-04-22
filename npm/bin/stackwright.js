#!/usr/bin/env node
// Shim: resolves the bundled Go binary for this platform and execs it,
// forwarding argv, stdio, and exit code verbatim.

const path = require("node:path");
const { spawnSync } = require("node:child_process");
const fs = require("node:fs");

const { platformTag, binaryName } = require("../scripts/platform.js");

const binDir = path.join(__dirname, "binary", platformTag());
const binPath = path.join(binDir, binaryName());

if (!fs.existsSync(binPath)) {
  console.error(`stackwright: binary not found at ${binPath}`);
  console.error(`stackwright: re-run 'npm install -g stackwright' to repair.`);
  process.exit(1);
}

const result = spawnSync(binPath, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  console.error("stackwright:", result.error.message);
  process.exit(1);
}
process.exit(result.status ?? 0);
