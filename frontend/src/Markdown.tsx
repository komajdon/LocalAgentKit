/**
 * Lightweight markdown renderer — no external deps.
 * Supports: fenced code blocks, inline code, bold, italic,
 * headers (h1-h3), unordered/ordered lists, blockquotes,
 * links, horizontal rules, and paragraphs.
 */
import { ReactNode, Component, ErrorInfo } from 'react'

function escapeHtml(s: string) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

/** Apply inline formatting: bold, italic, inline code, links */
function inlineToJSX(text: string, key: number): ReactNode {
  const tokens: ReactNode[] = []
  // Combined regex for **bold**, *italic*, `code`, [link](url)
  const re = /(\*\*(.+?)\*\*|\*(.+?)\*|`([^`]+)`|\[([^\]]+)\]\(([^)]+)\))/g
  let last = 0
  let m: RegExpExecArray | null
  let i = 0
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) tokens.push(<span key={i++}>{text.slice(last, m.index)}</span>)
    if (m[2] !== undefined) tokens.push(<strong key={i++}>{m[2]}</strong>)
    else if (m[3] !== undefined) tokens.push(<em key={i++}>{m[3]}</em>)
    else if (m[4] !== undefined) tokens.push(<code key={i++} className="md-inline-code">{m[4]}</code>)
    else if (m[5] !== undefined) {
      const href = m[6]
      const safeHref = /^https?:\/\/|^mailto:/i.test(href) ? href : '#'
      tokens.push(<a key={i++} href={safeHref} target="_blank" rel="noopener noreferrer">{m[5]}</a>)
    }
    last = m.index + m[0].length
  }
  if (last < text.length) tokens.push(<span key={i++}>{text.slice(last)}</span>)
  return tokens.length === 1 ? tokens[0] : <>{tokens}</>
}

type Block =
  | { t: 'code'; lang: string; body: string }
  | { t: 'heading'; level: 1 | 2 | 3; text: string }
  | { t: 'hr' }
  | { t: 'blockquote'; lines: string[] }
  | { t: 'ul'; items: string[] }
  | { t: 'ol'; items: string[] }
  | { t: 'para'; lines: string[] }

function parseBlocks(md: string): Block[] {
  const lines = md.split('\n')
  const blocks: Block[] = []
  let i = 0

  while (i < lines.length) {
    const line = lines[i]

    // Fenced code block
    const fenceMatch = line.match(/^```(\w*)/)
    if (fenceMatch) {
      const lang = fenceMatch[1] || ''
      const body: string[] = []
      i++
      while (i < lines.length && !lines[i].startsWith('```')) {
        body.push(lines[i])
        i++
      }
      i++ // skip closing ```
      blocks.push({ t: 'code', lang, body: body.join('\n') })
      continue
    }

    // Heading
    const hMatch = line.match(/^(#{1,3})\s+(.+)/)
    if (hMatch) {
      blocks.push({ t: 'heading', level: hMatch[1].length as 1 | 2 | 3, text: hMatch[2] })
      i++; continue
    }

    // Horizontal rule
    if (/^[-*_]{3,}$/.test(line.trim())) {
      blocks.push({ t: 'hr' })
      i++; continue
    }

    // Blockquote
    if (line.startsWith('> ')) {
      const bqLines: string[] = []
      while (i < lines.length && lines[i].startsWith('> ')) {
        bqLines.push(lines[i].slice(2))
        i++
      }
      blocks.push({ t: 'blockquote', lines: bqLines })
      continue
    }

    // Unordered list
    if (/^[-*+] /.test(line)) {
      const items: string[] = []
      while (i < lines.length && /^[-*+] /.test(lines[i])) {
        items.push(lines[i].slice(2))
        i++
      }
      blocks.push({ t: 'ul', items })
      continue
    }

    // Ordered list
    if (/^\d+\. /.test(line)) {
      const items: string[] = []
      while (i < lines.length && /^\d+\. /.test(lines[i])) {
        items.push(lines[i].replace(/^\d+\. /, ''))
        i++
      }
      blocks.push({ t: 'ol', items })
      continue
    }

    // Blank line — skip
    if (line.trim() === '') {
      i++; continue
    }

    // Paragraph — collect until blank line or block-level element
    const paraLines: string[] = []
    while (
      i < lines.length &&
      lines[i].trim() !== '' &&
      !/^(#{1,3} |```|> |[-*+] |\d+\. |[-*_]{3,}$)/.test(lines[i])
    ) {
      paraLines.push(lines[i])
      i++
    }
    if (paraLines.length > 0) blocks.push({ t: 'para', lines: paraLines })
  }

  return blocks
}

function renderBlock(block: Block, idx: number): ReactNode {
  switch (block.t) {
    case 'code':
      return (
        <div key={idx} className="md-code-block">
          {block.lang && <span className="md-code-lang">{block.lang}</span>}
          <pre><code>{block.body}</code></pre>
        </div>
      )
    case 'heading': {
      const content = inlineToJSX(block.text, idx)
      if (block.level === 1) return <h1 key={idx}>{content}</h1>
      if (block.level === 2) return <h2 key={idx}>{content}</h2>
      return <h3 key={idx}>{content}</h3>
    }
    case 'hr':
      return <hr key={idx} />
    case 'blockquote':
      return (
        <blockquote key={idx}>
          {block.lines.map((l, j) => <p key={j}>{inlineToJSX(l, j)}</p>)}
        </blockquote>
      )
    case 'ul':
      return (
        <ul key={idx}>
          {block.items.map((item, j) => <li key={j}>{inlineToJSX(item, j)}</li>)}
        </ul>
      )
    case 'ol':
      return (
        <ol key={idx}>
          {block.items.map((item, j) => <li key={j}>{inlineToJSX(item, j)}</li>)}
        </ol>
      )
    case 'para':
      return <p key={idx}>{inlineToJSX(block.lines.join(' '), idx)}</p>
  }
}

class MarkdownBoundary extends Component<{ children: ReactNode }, { crashed: boolean }> {
  state = { crashed: false }
  componentDidCatch(_err: Error, _info: ErrorInfo) { this.setState({ crashed: true }) }
  render() {
    if (this.state.crashed) return <pre className="md">{(this.props.children as any)?.props?.text ?? ''}</pre>
    return this.props.children
  }
}

function MarkdownInner({ text }: { text: string }) {
  const blocks = parseBlocks(text)
  return <div className="md">{blocks.map((b, i) => renderBlock(b, i))}</div>
}

export function Markdown({ text }: { text: string }) {
  return <MarkdownBoundary><MarkdownInner text={text} /></MarkdownBoundary>
}
