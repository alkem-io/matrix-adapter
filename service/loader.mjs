import { pathToFileURL } from 'url';
import { createRequire } from 'module';

// Register all path aliases from package.json
const require = createRequire(import.meta.url);
const { imports } = require('./package.json');

for (const [alias, path] of Object.entries(imports)) {
  const normalizedAlias = alias.replace('*', '');
  const normalizedPath = path.replace('*', '');

  // Register the alias
  await import('node:module').then(({ register }) => {
    register(normalizedPath, pathToFileURL(normalizedPath).href);
  });
}

// Load TypeScript files
const { load } = await import('ts-node/esm/transpile-only.js');
export { load };