#!/usr/bin/env node
// Build the divvy Go sidecar into desktop/binaries with the
// target-triple suffix Tauri's externalBin expects. The CLI runs this
// before bundling; dev.mjs runs it before `tauri dev`. Cross-platform:
// plain Node, no POSIX shell, so it works on Windows / macOS / Linux.
import { spawnSync } from 'node:child_process';
import { chmodSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const desktopDir = join(here, '..');
const repoRoot = join(desktopDir, '..');
const binDir = join(desktopDir, 'binaries');

const goExe = process.platform === 'win32' ? 'go.exe' : 'go';

function goEnv(name) {
  const r = spawnSync(goExe, ['env', name], { encoding: 'utf8' });
  if (r.status !== 0) {
    process.stderr.write(r.stderr || `go env ${name} failed\n`);
    process.exit(1);
  }
  return r.stdout.trim();
}

const goos = goEnv('GOOS');
const goarch = goEnv('GOARCH');

const triples = {
  'darwin/arm64': 'aarch64-apple-darwin',
  'darwin/amd64': 'x86_64-apple-darwin',
  'linux/amd64': 'x86_64-unknown-linux-gnu',
  'linux/arm64': 'aarch64-unknown-linux-gnu',
  'windows/amd64': 'x86_64-pc-windows-msvc',
  'windows/arm64': 'aarch64-pc-windows-msvc',
};
const triple = triples[`${goos}/${goarch}`];
if (!triple) {
  process.stderr.write(`unsupported sidecar target: ${goos}/${goarch}\n`);
  process.exit(1);
}

mkdirSync(binDir, { recursive: true });
const suffix = goos === 'windows' ? '.exe' : '';
const out = join(binDir, `divvy-sidecar-${triple}${suffix}`);
console.log(`building sidecar for ${triple} -> ${out}`);

const build = spawnSync(goExe, ['build', '-o', out, './cmd/divvy'], {
  cwd: repoRoot,
  stdio: 'inherit',
});
if (build.status !== 0) process.exit(build.status ?? 1);

// Executable bit defaults correctly on Windows; make sure of it elsewhere.
if (goos !== 'windows') chmodSync(out, 0o755);
console.log('sidecar ready:', out);
