import {defineConfig, Plugin} from 'vite'
import react from '@vitejs/plugin-react'

// Scripts may only come from the app itself, so injected markup cannot run
// code. Inline styles stay allowed because React sets style attributes. The
// policy is added to production builds only: the dev server needs inline
// scripts and a websocket.
const CSP = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "font-src 'self' data:",
  "connect-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
  "frame-src 'none'",
].join('; ')

const contentSecurityPolicy: Plugin = {
  name: 'content-security-policy',
  apply: 'build',
  transformIndexHtml: () => [
    {tag: 'meta', attrs: {'http-equiv': 'Content-Security-Policy', content: CSP}, injectTo: 'head-prepend'},
  ],
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), contentSecurityPolicy]
})
