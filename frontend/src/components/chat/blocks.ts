import type { ChatMsg, Item } from '../../types'

/** One tool call with its result, either of which may be missing. */
export interface Step { id: number; tool: string; args?: string; result?: string }

/** What the transcript draws. `index` is the position in the item list. */
export type Block =
  | { kind: 'msg'; id: number; index: number; msg: ChatMsg }
  | { kind: 'tools'; id: number; steps: Step[] }
  | { kind: 'error'; id: number; text: string }
  | { kind: 'status'; id: number; text: string }

/**
 * Groups consecutive tool items. Calls of one round arrive before their
 * results, so a result joins the oldest call of that tool still waiting.
 */
export function toBlocks(items: Item[]): Block[] {
  const blocks: Block[] = []
  items.forEach((item, index) => {
    if (item.type === 'msg') {
      blocks.push({ kind: 'msg', id: item.data.id, index, msg: item.data })
      return
    }
    if (item.type !== 'tool') {
      blocks.push({ kind: item.type, id: item.id, text: item.text })
      return
    }
    const t = item.data
    let group = blocks[blocks.length - 1]
    if (group?.kind !== 'tools') {
      group = { kind: 'tools', id: t.id, steps: [] }
      blocks.push(group)
    }
    if (t.kind === 'call') {
      group.steps.push({ id: t.id, tool: t.tool, args: t.body })
      return
    }
    const waiting = group.steps.find(s => s.tool === t.tool && s.result === undefined)
    if (waiting) waiting.result = t.body
    else group.steps.push({ id: t.id, tool: t.tool, result: t.body })
  })
  return blocks
}

/** Whether a result says the tool did not succeed. */
export function failed(result: string) {
  // Shell output starts with a line naming the isolation in effect.
  const body = result.startsWith('[isolation') ? result.slice(result.indexOf('\n') + 1) : result
  return /^(ERROR:|EXIT ERROR:|MCP call failed:|PERMISSION DENIED|CANCELED:)/.test(body)
}

const NAMES: Record<string, string> = {
  read_file: 'Read file', write_file: 'Write file', delete_file: 'Delete file', list_dir: 'List folder',
  search_files: 'Search files', shell: 'Run command', get_time: 'Check time',
  read_document: 'Read document', search_web: 'Search the web',
}

/** A readable name; MCP tools are "server · tool". */
export const toolName = (tool: string) => NAMES[tool] ?? tool.replace('__', ' · ')
