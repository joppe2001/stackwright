#!/usr/bin/env node
// Thin wrapper that just invokes the installed binary with --detect.
// Exposed so tooling / CI can run capability checks without the postinstall overhead.

const path = require("node:path");
const { execFileSync } = require("node:child_process");
const { platformTag, binaryName } = require("./platform.js");

const binPath = path.join(__dirname, "..", "bin", "binary", platformTag(), binaryName());

try {
  execFileSync(binPath, ["--detect"], { stdio: "inherit" });
} catch (err) {
  console.error("stackwright: --detect failed:", err.message);
  process.exit(1);
}
