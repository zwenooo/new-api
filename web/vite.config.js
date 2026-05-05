/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv, transformWithEsbuild } from 'vite';
import pkg from '@douyinfe/vite-plugin-semi';
const { vitePluginSemi } = pkg;

const rootDir = path.dirname(fileURLToPath(import.meta.url));

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, rootDir);
  // Use `localhost` as the default target to avoid IPv4/IPv6 and WSL port-forwarding
  // mismatches (e.g. `localhost` works but `127.0.0.1` hits another service).
  const proxyTarget = (env.VITE_PROXY_TARGET || '').trim() || 'http://localhost:3000';

  return {
  resolve: {
    alias: {
      // 兼容 Semi UI 新版 exports 限制，继续使用完整全局样式入口。
      '@douyinfe/semi-ui/dist/css/semi.css': path.join(
        rootDir,
        'node_modules/@douyinfe/semi-ui/dist/css/semi.css',
      ),
      // Work around package managers that may omit `dequal/lite/index.mjs`
      // (SWR imports `dequal/lite`, which Vite resolves via the `import` export condition).
      'dequal/lite': path.join(rootDir, 'node_modules/dequal/lite/index.js'),
    },
  },
  plugins: [
    {
      name: 'treat-js-files-as-jsx',
      async transform(code, id) {
        if (!/src\/.*\.js$/.test(id)) {
          return null;
        }

        // Use the exposed transform from vite, instead of directly
        // transforming with esbuild
        return transformWithEsbuild(code, id, {
          loader: 'jsx',
          jsx: 'automatic',
        });
      },
    },
    react(),
    vitePluginSemi({
      cssLayer: true,
    }),
  ],
  optimizeDeps: {
    force: true,
    esbuildOptions: {
      loader: {
        '.js': 'jsx',
        '.json': 'json',
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'react-core': ['react', 'react-dom', 'react-router-dom'],
          'semi-ui': ['@douyinfe/semi-icons', '@douyinfe/semi-ui'],
          tools: ['axios', 'history', 'marked'],
          'react-components': [
            'react-dropzone',
            'react-fireworks',
            'react-telegram-login',
            'react-toastify',
            'react-turnstile',
          ],
          i18n: [
            'i18next',
            'react-i18next',
            'i18next-browser-languagedetector',
          ],
        },
      },
    },
  },
  server: {
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: proxyTarget,
        changeOrigin: true,
        // 允许本地开发代理转发到证书链不完整的 HTTPS 后端
        secure: false,
      },
      '/mj': {
        target: proxyTarget,
        changeOrigin: true,
        // 允许本地开发代理转发到证书链不完整的 HTTPS 后端
        secure: false,
      },
      '/pg': {
        target: proxyTarget,
        changeOrigin: true,
        // 允许本地开发代理转发到证书链不完整的 HTTPS 后端
        secure: false,
      },
    },
  },
  };
});
