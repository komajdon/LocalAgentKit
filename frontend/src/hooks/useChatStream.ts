import { useCallback, useEffect, useRef, useState } from 'react'
import { GetRunState, RespondPermission } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { ChatEvent, ContextUsage, Item, PermReq } from '../types'
import type { PermissionDecision } from '../components/PermissionDialog'

const CHAT_EVENTS = ['started', 'thinking', 'chunk', 'assistant', 'tool_call', 'tool_result', 'context_usage', 'warning', 'done', 'stopped', 'error']
const FINAL_EVENTS = ['done', 'stopped', 'error']

let seq = 0
/** Allocates view item ids. Call it outside state updaters, which may run twice. */
export const uid = () => ++seq

interface Options {
  notify: (title: string, body: string) => void
  /** A run ended, whichever conversation it belonged to. */
  onRunEnd: () => void
}

/**
 * The transcript on screen and the run that feeds it. Events of other
 * conversations are ignored; events arriving while a conversation loads are
 * held and applied after its snapshot.
 */
export function useChatStream({ notify, onRunEnd }: Options) {
  const [items, setItems] = useState<Item[]>([])
  const [thinking, setThinking] = useState(false)
  const [runConv, setRunConv] = useState<string | null>(null)
  const [ctxUsage, setCtxUsage] = useState<ContextUsage | null>(null)
  const [permReq, setPermReq] = useState<PermReq | null>(null)

  const activeId = useRef<string | null>(null)
  const streamId = useRef<number | null>(null)
  const load = useRef({ token: 0, buffering: false, buffer: [] as [string, ChatEvent][] })
  const runStarted = useRef(0)
  // Subscriptions are made once; they read the latest callbacks through this.
  const options = useRef({ notify, onRunEnd })
  options.current = { notify, onRunEnd }

  const apply = useCallback((name: string, data: any) => {
    const finishStreaming = (id: number | null) => (it: Item): Item =>
      it.type === 'msg' && it.data.id === id ? { ...it, data: { ...it.data, streaming: false } } : it
    const takeStream = () => {
      const id = streamId.current
      streamId.current = null
      return id
    }
    const append = (item: Item) => setItems(prev => [...prev, item])

    switch (name) {
      case 'thinking':
        setThinking(true)
        break
      case 'chunk': {
        setThinking(false)
        const current = streamId.current
        if (current !== null) {
          setItems(prev => prev.map(it =>
            it.type === 'msg' && it.data.id === current ? { ...it, data: { ...it.data, text: it.data.text + data } } : it))
        } else {
          const id = uid()
          streamId.current = id
          append({ type: 'msg', data: { id, role: 'assistant', text: data, streaming: true } })
        }
        break
      }
      case 'assistant': {
        // The finished reply replaces the streamed text, which may hold tool calls.
        const id = takeStream()
        const text: string = data
        if (id === null) {
          if (text) append({ type: 'msg', data: { id: uid(), role: 'assistant', text, streaming: false } })
          break
        }
        setItems(prev => text
          ? prev.map(it => it.type === 'msg' && it.data.id === id ? { ...it, data: { ...it.data, text, streaming: false } } : it)
          : prev.filter(it => !(it.type === 'msg' && it.data.id === id)))
        break
      }
      case 'tool_call':
        append({ type: 'tool', data: { id: uid(), kind: 'call', tool: data.tool, body: data.args } })
        break
      case 'tool_result':
        append({ type: 'tool', data: { id: uid(), kind: 'result', tool: data.tool, body: data.result } })
        break
      case 'warning':
        append({ type: 'status', id: uid(), text: `Warning: ${data}` })
        break
      case 'context_usage':
        setCtxUsage(data)
        break
      case 'done': {
        setThinking(false)
        const id = takeStream()
        setItems(prev => prev.map(finishStreaming(id)))
        break
      }
      case 'stopped': {
        setThinking(false)
        const id = takeStream()
        const marker: Item = { type: 'status', id: uid(), text: 'Stopped by user' }
        setItems(prev => [...prev.map(finishStreaming(id)), marker])
        break
      }
      case 'error':
        setThinking(false)
        takeStream()
        append({ type: 'error', id: uid(), text: data })
        break
    }
  }, [])

  useEffect(() => {
    const offs = CHAT_EVENTS.map(name => EventsOn(`chat:${name}`, (e: ChatEvent) => {
      // Run bookkeeping applies whichever conversation is on screen.
      if (name === 'started') setRunConv(e.conversation_id)
      if (FINAL_EVENTS.includes(name)) {
        setRunConv(cur => (cur === e.conversation_id ? null : cur))
        if (name === 'error') options.current.notify('Agent error', e.data)
        else if (name === 'done' && runStarted.current && Date.now() - runStarted.current > 10_000) {
          options.current.notify('Agent finished', 'The task completed and is ready for you.')
        }
        runStarted.current = 0
        options.current.onRunEnd()
      }
      if (e.conversation_id !== activeId.current) return
      if (load.current.buffering) load.current.buffer.push([name, e])
      else apply(name, e.data)
    }))
    offs.push(EventsOn('chat:permission_request', (req: PermReq) => {
      setPermReq(req)
      options.current.notify('Permission needed', `The agent wants to run "${req.tool}" — your approval is required.`)
    }))
    offs.push(EventsOn('chat:permission_closed', (id: string) => setPermReq(cur => (cur?.id === id ? null : cur))))
    GetRunState().then(r => { if (r.run_id) setRunConv(r.conversation_id) }).catch(() => {})
    return () => offs.forEach(off => off())
  }, [apply])

  /** Call before loading a conversation; pass the token to commitLoad. */
  const beginLoad = useCallback((id: string) => {
    load.current.buffering = true
    load.current.buffer = []
    activeId.current = id
    return ++load.current.token
  }, [])

  const isStale = useCallback((token: number) => token !== load.current.token, [])

  const abortLoad = useCallback((token: number) => {
    if (token === load.current.token) load.current.buffering = false
  }, [])

  /** Shows a loaded transcript, then the events its snapshot did not include. */
  const commitLoad = useCallback((token: number, loaded: Item[], running: boolean, snapshotSeq: number) => {
    if (token !== load.current.token) return
    setThinking(false)
    setCtxUsage(null)
    const last = loaded[loaded.length - 1]
    streamId.current = running && last?.type === 'msg' && last.data.streaming ? last.data.id : null
    setItems(loaded)
    load.current.buffering = false
    for (const [name, e] of load.current.buffer) {
      if (e.seq > snapshotSeq) apply(name, e.data)
    }
    load.current.buffer = []
  }, [apply])

  const clear = useCallback(() => {
    activeId.current = null
    streamId.current = null
    setItems([])
    setThinking(false)
    setCtxUsage(null)
  }, [])

  /** Optimistic state for a run that is about to start. */
  const startedRun = useCallback((id: string) => {
    setRunConv(id)
    setThinking(true)
    streamId.current = null
    runStarted.current = Date.now()
  }, [])

  const runRejected = useCallback((id: string, error?: string) => {
    if (!error) return
    setRunConv(cur => (cur === id ? null : cur))
    setThinking(false)
    if (activeId.current === id) {
      const item: Item = { type: 'error', id: uid(), text: error }
      setItems(prev => [...prev, item])
    }
    GetRunState().then(r => setRunConv(r.run_id ? r.conversation_id : null)).catch(() => {})
  }, [])

  const answerPermission = useCallback((decision: PermissionDecision) => {
    if (!permReq) return
    RespondPermission(permReq.id, decision)
    setPermReq(null)
  }, [permReq])

  return {
    items, setItems, thinking, runConv, ctxUsage, setCtxUsage, permReq, answerPermission,
    beginLoad, isStale, abortLoad, commitLoad, clear, startedRun, runRejected,
  }
}
