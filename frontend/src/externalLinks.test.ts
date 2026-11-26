// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'

const open = vi.fn()
vi.mock('../wailsjs/runtime/runtime', () => ({ BrowserOpenURL: (url: string) => open(url) }))

const { installExternalLinkHandler } = await import('./externalLinks')
installExternalLinkHandler()

// click returns whether the view would have navigated.
function click(href: string, type = 'click') {
  document.body.innerHTML = `<a href="${href}"><span>x</span></a>`
  return document.querySelector('span')!.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true }))
}

describe('link handling', () => {
  beforeEach(() => open.mockClear())

  it.each(['https://example.com/a', 'mailto:a@example.com'])('opens %s outside the app', href => {
    expect(click(href)).toBe(false)
    expect(open).toHaveBeenCalledWith(href)
  })

  it.each(['javascript:alert(1)', 'file:///etc/passwd', '/local', '#', 'https-evil:x'])('swallows %s', href => {
    expect(click(href)).toBe(false)
    expect(open).not.toHaveBeenCalled()
  })

  it('also covers middle clicks', () => {
    expect(click('https://example.com', 'auxclick')).toBe(false)
  })
})
