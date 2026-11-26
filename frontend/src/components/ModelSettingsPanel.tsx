import { useEffect, useState } from 'react'
import { DescribeModel, GetModelSettings, RefreshModels, SaveModelSettings, TestConnection } from '../../wailsjs/go/main/App'

interface Override { context_limit: number; tool_calling: string }
interface Settings { timeout_seconds: number; overrides: Record<string, Override> }
interface Profile {
  model: string; context_limit: number; tool_calling: string
  detected: { native_tools: boolean | null; context_window: number }
}

interface Props {
  provider: string
  model: string
  onModels: (models: string[]) => void
}

/** Connection test, model refresh and per-model overrides. Acts on the saved provider settings. */
export function ModelSettingsPanel({ provider, model, onModels }: Props) {
  const [settings, setSettings] = useState<Settings | null>(null)
  const [profile, setProfile] = useState<Profile | null>(null)
  const [status, setStatus] = useState('')
  const [busy, setBusy] = useState(false)
  const key = `${provider}/${model}`
  const override = settings?.overrides[key] ?? { context_limit: 0, tool_calling: '' }

  useEffect(() => { GetModelSettings().then(s => setSettings(s as any)).catch(e => setStatus(String(e))) }, [])
  useEffect(() => { setProfile(null) }, [key])

  const run = async (action: () => Promise<string>) => {
    setBusy(true); setStatus('Working…')
    try { setStatus(await action()) } catch (e) { setStatus(String(e)) } finally { setBusy(false) }
  }

  const save = (next: Settings) => {
    setSettings(next)
    SaveModelSettings(next as any).then(err => setStatus(err || 'Saved. Applies from the next run.'))
  }
  const setOverride = (change: Partial<Override>) =>
    settings && save({ ...settings, overrides: { ...settings.overrides, [key]: { ...override, ...change } } })

  return (
    <div className="field" aria-label="Connection and model">
      <span className="field-label">Connection and model</span>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <button className="btn secondary" disabled={busy} onClick={() => run(async () => {
          const r = await TestConnection()
          return r.ok ? `Connected in ${r.millis} ms; ${r.models} models.` : `Failed: ${r.error}`
        })}>Test connection</button>
        <button className="btn secondary" disabled={busy} onClick={() => run(async () => {
          const r = await RefreshModels()
          onModels(r.models ?? [])
          return r.error ? `Failed: ${r.error}` : `${(r.models ?? []).length} models.`
        })}>Refresh models</button>
        <button className="btn secondary" disabled={busy || !model} onClick={() => run(async () => {
          setProfile(await DescribeModel(model) as any)
          return ''
        })}>Detect capabilities</button>
      </div>
      {status && <div className="field-hint" role="status">{status}</div>}
      {profile && (
        <div className="field-hint">
          Context window: {profile.detected.context_window || 'not reported'} · Native tools:{' '}
          {profile.detected.native_tools === null ? 'not reported' : profile.detected.native_tools ? 'yes' : 'no'} · Will use
          {' '}{profile.context_limit} tokens, {profile.tool_calling} tool calling
        </div>
      )}
      {settings && model && (
        <div className="usage-grid">
          <label>Context limit for {model} (0 = automatic)
            <input type="number" min={0} step={1024} value={override.context_limit}
              onChange={e => setOverride({ context_limit: Number(e.target.value) })} />
          </label>
          <label>Tool calling for {model}
            <select value={override.tool_calling} onChange={e => setOverride({ tool_calling: e.target.value })}>
              <option value="">Automatic</option>
              <option value="native">Native</option>
              <option value="text">Text protocol</option>
            </select>
          </label>
          <label>Provider timeout, seconds (0 = default)
            <input type="number" min={0} max={3600} value={settings.timeout_seconds}
              onChange={e => save({ ...settings, timeout_seconds: Number(e.target.value) })} />
          </label>
        </div>
      )}
      <div className="field-hint">These act on the saved provider and address; save first after changing them.</div>
    </div>
  )
}
