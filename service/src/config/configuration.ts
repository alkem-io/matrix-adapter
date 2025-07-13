import { readFileSync } from 'fs';
import { dirname,join } from 'path';
import { fileURLToPath } from 'url';

import YAML from 'yaml';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const YAML_CONFIG_FILENAME = 'matrix-adapter.yml';

export default () => {
  const rawYaml = readFileSync(
    join(__dirname, '../../', YAML_CONFIG_FILENAME),
    'utf8'
  );

  const doc = YAML.parseDocument(rawYaml);
  const envConfig = process.env;

  YAML.visit(doc, {
    Scalar(key, node) {
      if (node.type === 'PLAIN') {
        node.value = buildYamlNodeValue(node.value, envConfig);
      }
    },
  });

  const config = doc.toJSON() as Record<string, any>;
  return config;
};

function buildYamlNodeValue(nodeValue: any, envConfig: any) {
  let updatedNodeValue = nodeValue;
  const key = `${nodeValue}`;
  const regex = '\\${(.*)}:?(.*)';
  const found = key.match(regex);
  if (found) {
    const envVariableKey = found[1];
    const envVariableDefaultValue = found[2];

    updatedNodeValue = envConfig[envVariableKey] ?? envVariableDefaultValue;

    if (typeof updatedNodeValue === 'string' && updatedNodeValue.toLowerCase() === 'true') return true;
    if (typeof updatedNodeValue === 'string' && updatedNodeValue.toLowerCase() === 'false') return false;
  }

  return updatedNodeValue;
}
