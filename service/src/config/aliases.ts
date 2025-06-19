import moduleAlias from 'module-alias';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const rootPath = path.resolve(__dirname, '..', '..', 'dist');
const rootCorePath = path.join(rootPath, 'core');
const rootCommonPath = path.join(rootPath, 'common');
moduleAlias.addAliases({
  '@src': rootPath,
  '@common': path.join(rootCommonPath),
  '@core': path.join(rootCorePath),
  '@config': path.join(rootCorePath, 'config'),
});
