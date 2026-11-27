import { useEffect, useRef } from 'react'
import { StopAgent } from '../../../wailsjs/go/main/App'
import { activate } from '../../activate'
import type { useSpeech } from '../../hooks/useSpeech'
import type { ContextUsage } from '../../types'
import { Icon } from '../Icon'

function ContextMeter({ usage }: { usage: ContextUsage }) {
  const pct = Math.min(100, Math.round(usage.used / usage.limit * 100))
  const approx = usage.estimated ? '~' : ''
  const title = `Context: ${approx}${usage.used.toLocaleString()} of ${usage.limit.toLocaleString()} tokens`
    + (usage.estimated ? ' (estimated)' : '')
    + (usage.total ? `\n${usage.total.toLocaleString()} tokens used in this conversation` : '')
    + (usage.omitted ? `\n${usage.omitted} older messages are not sent`
      + (usage.summarised ? `; ${usage.summarised} are summarised` : '') + ' (history is kept)' : '')
  return (
    <span className={`ctx-meter ${pct >= 80 ? 'warn' : ''}`} title={title}>
      <span className="ctx-track" aria-hidden="true"><span className="ctx-fill" style={{ width: pct + '%' }} /></span>
      {approx}{pct}% context
    </span>
  )
}

interface Props {
  value: string
  onChange: (value: string) => void
  onSend: () => void
  running: boolean
  /** A run is active in some conversation. */
  busy: boolean
  usage: ContextUsage | null
  speech: ReturnType<typeof useSpeech>
}

export function Composer({ value, onChange, onSend, running, busy, usage, speech }: Props) {
  const { whisperReady, recording, transcribing, whisperStatus, sttError, setSttError, startRecording, stopRecording } = speech
  const box = useRef<HTMLTextAreaElement>(null)

  // Grow with the text, and shrink again after sending.
  useEffect(() => {
    const el = box.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 160) + 'px'
  }, [value])

  const canRecord = whisperReady && !transcribing && !recording
  return (
    <>
      {sttError && <div className="stt-error" {...activate(() => setSttError(''), `Dismiss: ${sttError}`)}>{sttError}</div>}
      {whisperStatus && <div className="whisper-status">{whisperStatus}</div>}
      <div className="input-bar">
        <div className="input-shell">
          <textarea ref={box} rows={1} value={value} aria-label="Message" disabled={transcribing}
            placeholder={transcribing ? 'Transcribing…' : 'Message the agent…'}
            onChange={e => onChange(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); onSend() }
            }} />
        </div>
        <div className="input-tools">
          <span className="input-hint"><kbd>Enter</kbd> send · <kbd>Shift</kbd> <kbd>Enter</kbd> new line</span>
          {usage && <ContextMeter usage={usage} />}
          <button type="button" disabled={running || transcribing} aria-label="Hold to record"
            className={`mic-btn ${recording ? 'recording' : ''} ${!whisperReady ? 'unavailable' : ''}`}
            onMouseDown={canRecord ? startRecording : undefined}
            onMouseUp={recording ? () => stopRecording() : undefined}
            onMouseLeave={recording ? () => stopRecording() : undefined}
            onTouchStart={canRecord ? e => { e.preventDefault(); startRecording() } : undefined}
            onTouchEnd={recording ? () => stopRecording() : undefined}
            onClick={!whisperReady ? () => setSttError('Whisper not found. Install with: pip install openai-whisper (requires ffmpeg too)') : undefined}
            title={!whisperReady ? 'Voice input is not installed' : recording ? 'Release to transcribe' : 'Hold to record'}>
            <Icon name="mic" />
          </button>
          {running
            ? <button className="send-btn stop" onClick={() => StopAgent()} title="Stop" aria-label="Stop"><Icon name="stop" /></button>
            : <button className="send-btn" onClick={onSend} disabled={!value.trim() || transcribing || busy} title="Send" aria-label="Send"><Icon name="send" /></button>}
        </div>
      </div>
    </>
  )
}
