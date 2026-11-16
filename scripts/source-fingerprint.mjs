import { createReadStream, readFileSync, statSync } from 'node:fs';
import { Readable } from 'node:stream';
import { pipeline } from 'node:stream/promises';

// Keep the shell's sorted path/NUL/content/NUL encoding and bound output buffering.
async function* sourceBytes() {
  const names = readFileSync(0);
  for (let start = 0; start < names.length;) {
    let end = names.indexOf(10, start);
    if (end < 0) end = names.length;
    const name = names.subarray(start, end);
    start = end + 1;
    if (!name.length) continue;
    const file = name;
    let info;
    try {
      info = statSync(file);
    } catch (error) {
      if (error.code === 'ENOENT' || error.code === 'ENOTDIR') continue;
      throw error;
    }
    if (!info.isFile()) continue;
    yield name;
    yield Buffer.from([0]);
    yield* createReadStream(file);
    yield Buffer.from([0]);
  }
}

await pipeline(Readable.from(sourceBytes()), process.stdout);
