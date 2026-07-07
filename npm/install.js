#!/usr/bin/env node
"use strict";

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");
const http = require("http");

const REPO = "lzy1102/openaide";
const BIN_DIR = path.join(__dirname, "bin");

function getPlatform() {
  const os = process.platform;
  if (os === "darwin") return "darwin";
  if (os === "linux") return "linux";
  if (os === "win32") return "windows";
  throw new Error(`Unsupported platform: ${os}`);
}

function getArch() {
  const arch = process.arch;
  if (arch === "x64") return "amd64";
  if (arch === "arm64") return "arm64";
  throw new Error(`Unsupported architecture: ${arch}`);
}

function getBinaryName(platform) {
  return platform === "windows" ? "openaide.exe" : "openaide";
}

function getDownloadUrl(version, platform, arch) {
  const binaryName = getBinaryName(platform);
  const ext = platform === "windows" ? ".exe" : "";
  const filename = `openaide-${platform}-${arch}${ext}`;
  return `https://github.com/${REPO}/releases/download/v${version}/${filename}`;
}

function download(url) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith("https") ? https : http;
    client
      .get(url, { headers: { "User-Agent": "openaide-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return download(res.headers.location).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode}: ${url}`));
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function main() {
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  const version = pkg.version;
  const platform = getPlatform();
  const arch = getArch();
  const binaryName = getBinaryName(platform);

  const url = getDownloadUrl(version, platform, arch);
  const binPath = path.join(BIN_DIR, binaryName);

  if (fs.existsSync(binPath)) {
    return;
  }

  fs.mkdirSync(BIN_DIR, { recursive: true });

  console.log(`openaide: downloading v${version} for ${platform}/${arch}...`);
  try {
    const data = await download(url);
    fs.writeFileSync(binPath, data);
    if (platform !== "windows") {
      fs.chmodSync(binPath, 0o755);
    }
    console.log(`openaide: installed to ${binPath}`);
  } catch (err) {
    console.error(`openaide: download failed: ${err.message}`);
    console.error(`  You can manually download from: https://github.com/${REPO}/releases`);
    process.exit(1);
  }
}

main();
