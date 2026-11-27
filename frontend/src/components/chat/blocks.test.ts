import { describe, expect, it } from 'vitest'
import type { Item } from '../../types'
import { failed, toBlocks, toolName } from './blocks'

const tool = (id: number, kind: 'call' | 'result', name: string, body: string): Item =>
  ({ type: 'tool', data: { id, kind, tool: name, body } })

describe('transcript blocks', () => {
  it('pairs parallel calls with their results in order', () => {
    const blocks = toBlocks([
      { type: 'msg', data: { id: 1, role: 'user', text: 'q' } },
      tool(2, 'call', 'read_file', 'path=a'), tool(3, 'call', 'read_file', 'path=b'),
      tool(4, 'result', 'read_file', 'A'), tool(5, 'result', 'read_file', 'B'),
      tool(6, 'result', 'shell', 'PERMISSION DENIED by user'),
      { type: 'msg', data: { id: 7, role: 'assistant', text: 'a' } },
    ])
    expect(blocks.map(b => b.kind)).toEqual(['msg', 'tools', 'msg'])
    const steps = blocks[1].kind === 'tools' ? blocks[1].steps : []
    expect(steps.map(s => [s.args, s.result])).toEqual([['path=a', 'A'], ['path=b', 'B'], [undefined, 'PERMISSION DENIED by user']])
    // Branching needs the position in the item list, not in the block list.
    expect(blocks[2].kind === 'msg' && blocks[2].index).toBe(6)
  })

  it('recognises failures, including sandboxed shell output', () => {
    expect(failed('ERROR: x')).toBe(true)
    expect(failed('[isolation: bubblewrap, network blocked]\nEXIT ERROR: exit status 1')).toBe(true)
    expect(failed('[isolation: none (off)]\nall good')).toBe(false)
    expect(failed('the file says ERROR: in the middle')).toBe(false)
    expect(toolName('db__query')).toBe('db · query')
  })
})
