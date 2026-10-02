import { defineConfig, globalIgnores } from 'eslint/config';
import expoFlatConfig from 'eslint-config-expo/flat.js';

// 与 apps/web-client 的 flat config 同构：defineConfig + globalIgnores，
// 规则集用 Expo 官方 flat 数组（core/typescript/react/expo 四组）。
export default defineConfig([
  globalIgnores(['dist/', '.expo/']),
  ...expoFlatConfig,
]);
