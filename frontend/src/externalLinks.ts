import { BrowserOpenURL } from '../wailsjs/runtime/runtime'

// Links must never navigate the app's own view: a page loaded there could call
// the Go bindings. Web and mail links open in the system browser instead.
const external = /^(https?:|mailto:)/i

export function handleLinkClick(event: MouseEvent) {
  const anchor = (event.target as Element | null)?.closest?.('a[href]')
  if (!anchor) return
  event.preventDefault()
  const href = anchor.getAttribute('href') || ''
  if (external.test(href)) BrowserOpenURL(href)
}

export function installExternalLinkHandler(doc: Document = document) {
  doc.addEventListener('click', handleLinkClick, true)
  doc.addEventListener('auxclick', handleLinkClick, true)
}
