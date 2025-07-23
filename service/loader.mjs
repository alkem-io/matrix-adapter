import { createRequire } from 'module';
import { pathToFileURL } from 'url';

// Register all path aliases from package.json
const require = createRequire(import.meta.url);
const { imports } = require('./package.json');

for (const [path] of Object.entries(imports)) {
  const normalizedPath = path.replace('*', '');

  // Register the alias
  await import('node:module').then(({ register }) => {
    register(normalizedPath, pathToFileURL(normalizedPath).href);
  });
}

// Load TypeScript files
const { load } = await import('ts-node/esm/transpile-only.js');
export { load };