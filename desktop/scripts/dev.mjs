#!/usr/bin/env node
// `tauri dev` hook (cross-platform): make sure the Go sidecar binary
// exists, then start Vite. The Vite dev server proxies /api to the
// sidecar's fixed dev port (must stay in sync with DEFAULT_DEV_SIDECAR_PORT
// in src/sidecar.rs).
import { spawn, spawnSync } from 'node:child_process';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const webDir = join(here, '..', '..', 'web');

const build = spawnSync(process.execPath, [join(here, 'build-sidecar.mjs')], {
  stdio: 'inherit',
});
if (build.status !== 0) process.exit(build.status ?? 1);

process.env.VITE_DEV_TARGET =
  process.env.VITE_DEV_TARGET || 'http://127.0.0.1:8799';

// npm ships as npm.cmd on Windows; execPath (the real node binary) avoids
// relying on PATHEXT resolution.
const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm';
const child = spawn(npm, ['run', 'dev'], {
  cwd: webDir,
  stdio: 'inherit',
  env: process.env,
});

// Forward termination so Ctrl+C takes Vite (and its children) down with us.
for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => child.kill(sig));
}
child.on('exit', (code, signal) => {
  process.exit(code ?? (signal ? 1 : 0));
});
