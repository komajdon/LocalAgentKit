// @vitest-environment jsdom
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { Markdown } from './Markdown'

afterEach(cleanup)

const html = (text: string) => render(<Markdown text={text} />).container

describe('Markdown with untrusted model output', () => {
  it('shows raw HTML as text instead of creating elements', () => {
    const c = html('<img src=x onerror=alert(1)> <script>alert(2)</script>\n\n```\n<iframe></iframe>\n```')
    expect(c.querySelector('img, script, iframe')).toBeNull()
    expect(c.textContent).toContain('<script>alert(2)</script>')
  })

  it.each([
    'javascript:alert(1)', 'JavaScript:alert(1)', 'data:text/html,x', 'file:///etc/passwd',
    'wails://wails/', ' https://padded.example', 'vbscript:x',
  ])('does not link to %s', url => {
    expect(html(`[go](${url})`).querySelector('a')?.getAttribute('href')).toBe('#')
  })

  it('keeps web and mail links', () => {
    const links = [...html('[a](https://example.com) [b](mailto:a@example.com)').querySelectorAll('a')]
    expect(links.map(a => a.getAttribute('href'))).toEqual(['https://example.com', 'mailto:a@example.com'])
  })
})
