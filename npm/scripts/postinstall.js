#!/usr/bin/env node
// Runs after `npm install -g stackwright`:
//   1. Resolves the correct binary for this platform.
//   2. Downloads it from the matching GitHub release.
//   3. Marks it executable and places it under bin/binary/<platform>/.
//   4. Runs the binary with --detect and prints the capability report.
//
// Failure is fatal — we exit(1) so npm surfaces the error rather than leaving
// the user with a silently-broken install.

const fs = require("node:fs");
const path = require("node:path");
const https = require("node:https");
const { execFileSync } = require("node:child_process");
const zlib = require("node:zlib");
const tar = require("node:stream/consumers");

const { platformTag, binaryName } = require("./platform.js");

const REPO_OWNER = "joppe2001";
const REPO_NAME  = "stackwright";

async function main() {
  const pkg = require("../package.json");
  const version = pkg.version;

  const tag = platformTag();
  const bin = binaryName();

  const binDir = path.join(__dirname, "..", "bin", "binary", tag);
  const binPath = path.join(binDir, bin);

  // Skip download if the binary is already in place (useful for local dev /
  // repeat installs from the same tarball).
  if (fs.existsSync(binPath)) {
    console.log(`stackwright: binary already present at ${binPath}`);
  } else if (process.env.STACKWRIGHT_SKIP_DOWNLOAD === "1") {
    // Explicit opt-out for CI / local dev against a yet-to-be-uploaded release.
    console.log(`stackwright: STACKWRIGHT_SKIP_DOWNLOAD=1 — skipping binary download`);
    console.log(`stackwright: place the binary at ${binPath} before running the CLI`);
  } else {
    fs.mkdirSync(binDir, { recursive: true });
    const url = `https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${version}/stackwright-${tag}.tar.gz`;
    console.log(`stackwright: downloading ${url}`);
    await downloadAndExtract(url, binDir);
    fs.chmodSync(binPath, 0o755);
    console.log(`stackwright: installed to ${binPath}`);
  }

  // Capability report.
  try {
    console.log();
    execFileSync(binPath, ["--detect"], { stdio: "inherit" });
  } catch (err) {
    console.error("stackwright: --detect failed:", err.message);
    process.exit(1);
  }
}

function downloadAndExtract(url, destDir) {
  return new Promise((resolve, reject) => {
    https.get(url, { headers: { "user-agent": "stackwright-postinstall" } }, (res) => {
      // Follow one redirect (GitHub release asset → S3).
      if (res.statusCode === 301 || res.statusCode === 302) {
        return resolve(downloadAndExtract(res.headers.location, destDir));
      }
      if (res.statusCode !== 200) {
        return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
      }
      const gunzip = zlib.createGunzip();
      res.pipe(gunzip);
      // Minimal tar extraction — we only need the single binary from the archive.
      // Node 18+ ships no built-in tar parser, so we pipe through to a tmpfile
      // and shell out to `tar`. macOS and Linux both ship bsdtar/gnutar.
      const tmpFile = path.join(destDir, "_download.tar");
      const out = fs.createWriteStream(tmpFile);
      gunzip.pipe(out);
      out.on("finish", () => {
        try {
          execFileSync("tar", ["-xf", tmpFile, "-C", destDir], { stdio: "inherit" });
          fs.unlinkSync(tmpFile);
          resolve();
        } catch (err) {
          reject(err);
        }
      });
      out.on("error", reject);
    }).on("error", reject);
  });
}

main().catch((err) => {
  console.error("stackwright postinstall failed:", err.message);
  process.exit(1);
});
