import { readdir, readFile, writeFile } from "node:fs/promises";
import { gzipSync } from "node:zlib";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
async function compress(dir) {
  for (const item of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, item.name);
    if (item.isDirectory()) await compress(p);
    else if (/\.(js|css)$/.test(item.name))
      await writeFile(p + ".gz", gzipSync(await readFile(p), { level: 9 }));
  }
}
await compress(
  fileURLToPath(new URL("../../internal/web/dist/", import.meta.url)),
);
