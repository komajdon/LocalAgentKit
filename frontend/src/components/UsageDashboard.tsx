import { useCallback, useEffect, useMemo, useState } from 'react'
import { GetUsageReport, GetUsageSettings, SaveUsageSettings } from '../../wailsjs/go/main/App'

export interface Totals {
  requests: number; estimated_requests: number
  input: number; output: number; cached: number
  cost: number; unpriced_requests: number
}
export interface Entry extends Totals { day: string; provider: string; model: string; conversation_id: string }
interface Price { provider: string; model: string; input_per_million: number; output_per_million: number; cached_per_million: number }
interface Limits {
  run_tokens: number; run_requests: number; run_cost: number
  daily_tokens: number; daily_cost: number; monthly_tokens: number; monthly_cost: number
  warn_percent: number; hard_period_limits: boolean
}
interface Settings { currency: string; prices: Price[]; limits: Limits }
interface Filter { provider: string; model: string; conversation_id: string; from: string; to: string }

const NO_FILTER: Filter = { provider: '', model: '', conversation_id: '', from: '', to: '' }
const LIMIT_FIELDS: [Exclude<keyof Limits, 'hard_period_limits'>, string][] = [
  ['run_tokens', 'Tokens per run'], ['run_requests', 'Requests per run'], ['run_cost', 'Cost per run'],
  ['daily_tokens', 'Tokens per day'], ['daily_cost', 'Cost per day'],
  ['monthly_tokens', 'Tokens per month'], ['monthly_cost', 'Cost per month'],
  ['warn_percent', 'Warn at % of limit'],
]

/** Cost is only what the user's prices cover, so say when some is missing. */
export function costLabel(t: Pick<Totals, 'cost' | 'requests' | 'unpriced_requests'>, currency: string) {
  if (t.requests > 0 && t.unpriced_requests === t.requests) return 'no price set'
  const amount = `≈ ${t.cost.toFixed(4)} ${currency}`
  return t.unpriced_requests > 0 ? `${amount} + ${t.unpriced_requests} unpriced` : amount
}

interface Props {
  conversations: { id: string; title: string }[]
  onClose: () => void
}

export function UsageDashboard({ conversations, onClose }: Props) {
  const [filter, setFilter] = useState<Filter>(NO_FILTER)
  const [report, setReport] = useState<{ entries: Entry[]; total: Totals } | null>(null)
  const [all, setAll] = useState<Entry[]>([])
  const [settings, setSettings] = useState<Settings | null>(null)
  const [message, setMessage] = useState('')

  const fail = useCallback((err: unknown) => setMessage(String(err)), [])

  useEffect(() => {
    GetUsageSettings().then(s => setSettings(s as any)).catch(fail)
    GetUsageReport(NO_FILTER as any).then(r => setAll((r as any).entries)).catch(fail)
  }, [fail])

  useEffect(() => {
    let current = true
    GetUsageReport(filter as any).then(r => { if (current) setReport(r as any) }).catch(fail)
    return () => { current = false }
  }, [filter, fail])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const options = useMemo(() => ({
    providers: [...new Set(all.map(e => e.provider))].sort(),
    models: [...new Set(all.map(e => e.model))].sort(),
    conversations: [...new Set(all.map(e => e.conversation_id))],
  }), [all])
  const title = (id: string) => conversations.find(c => c.id === id)?.title ?? '(deleted conversation)'

  const patch = (change: Partial<Settings>) => setSettings(s => (s ? { ...s, ...change } : s))
  const patchPrice = (i: number, change: Partial<Price>) =>
    settings && patch({ prices: settings.prices.map((p, j) => (j === i ? { ...p, ...change } : p)) })

  const save = async () => {
    if (!settings) return
    const err = await SaveUsageSettings(settings as any)
    setMessage(err || 'Saved. Applies from the next run.')
  }

  const select = (key: keyof Filter, label: string, values: string[], show = (v: string) => v) => (
    <label>{label}
      <select value={filter[key]} onChange={e => setFilter(f => ({ ...f, [key]: e.target.value }))}>
        <option value="">All</option>
        {values.map(v => <option key={v} value={v}>{show(v)}</option>)}
      </select>
    </label>
  )

  const currency = settings?.currency ?? ''
  const total = report?.total

  return (
    <div className="overlay" role="dialog" aria-modal="true" aria-label="Usage">
      <div className="modal wide">
        <div className="modal-hd">
          <div><h2>Usage</h2><p>Recorded from now on; costs are computed from the prices you enter below.</p></div>
        </div>
        <div className="modal-body">
          <div className="usage-filters">
            {select('provider', 'Provider', options.providers)}
            {select('model', 'Model', options.models)}
            {select('conversation_id', 'Conversation', options.conversations, title)}
            {(['from', 'to'] as const).map(k => (
              <label key={k}>{k === 'from' ? 'From' : 'To'}
                <input type="date" value={filter[k]} onChange={e => setFilter(f => ({ ...f, [k]: e.target.value }))} />
              </label>
            ))}
          </div>

          {!report && <div className="field-hint" role="status">Loading…</div>}
          {report && report.entries.length === 0 && <div className="field-hint">No usage recorded for this selection.</div>}
          {total && total.requests > 0 && (
            <>
              <div className="field-hint" role="status">
                {total.requests} requests ({total.estimated_requests} with estimated token counts) ·{' '}
                {total.input.toLocaleString()} input ({total.cached.toLocaleString()} cached) ·{' '}
                {total.output.toLocaleString()} output · {costLabel(total, currency)}
              </div>
              <div className="usage-table-wrap">
                <table className="usage-table">
                  <thead><tr>
                    <th>Date</th><th>Provider</th><th>Model</th><th>Conversation</th>
                    <th>Requests</th><th>Input</th><th>Output</th><th>Cost</th>
                  </tr></thead>
                  <tbody>
                    {report!.entries.slice(0, 200).map(e => (
                      <tr key={[e.day, e.provider, e.model, e.conversation_id].join('|')}>
                        <td>{e.day}</td><td>{e.provider}</td><td>{e.model}</td><td>{title(e.conversation_id)}</td>
                        <td>{e.requests}</td>
                        <td>{e.estimated_requests ? '~' : ''}{e.input.toLocaleString()}</td>
                        <td>{e.estimated_requests ? '~' : ''}{e.output.toLocaleString()}</td>
                        <td>{costLabel(e, currency)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {report!.entries.length > 200 && <div className="field-hint">Showing 200 rows; narrow the filter to see the rest.</div>}
            </>
          )}

          {settings && (
            <>
              <div className="field">
                <span className="field-label">Prices per million tokens</span>
                <label>Currency <input value={settings.currency} onChange={e => patch({ currency: e.target.value })} /></label>
                {settings.prices.length > 0 && (
                  <div className="usage-table-wrap">
                    <table className="usage-table">
                      <thead><tr><th>Provider</th><th>Model or *</th><th>Input</th><th>Output</th><th>Cached</th><th /></tr></thead>
                      <tbody>
                        {settings.prices.map((p, i) => (
                          <tr key={i}>
                            {(['provider', 'model'] as const).map(k => (
                              <td key={k}><input aria-label={k} value={p[k]} onChange={e => patchPrice(i, { [k]: e.target.value })} /></td>
                            ))}
                            {(['input_per_million', 'output_per_million', 'cached_per_million'] as const).map(k => (
                              <td key={k}><input aria-label={k} type="number" min={0} step="any" value={p[k]}
                                onChange={e => patchPrice(i, { [k]: Number(e.target.value) })} /></td>
                            ))}
                            <td><button className="btn secondary" onClick={() => patch({ prices: settings.prices.filter((_, j) => j !== i) })}>Remove</button></td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
                <button className="btn secondary" onClick={() => patch({ prices: [...settings.prices, { provider: '', model: '', input_per_million: 0, output_per_million: 0, cached_per_million: 0 }] })}>
                  Add price
                </button>
              </div>

              <div className="field">
                <span className="field-label">Limits (0 means none)</span>
                <div className="usage-grid">
                  {LIMIT_FIELDS.map(([key, label]) => (
                    <label key={key}>{label}
                      <input type="number" min={0} step="any" value={settings.limits[key]}
                        onChange={e => patch({ limits: { ...settings.limits, [key]: Number(e.target.value) } })} />
                    </label>
                  ))}
                </div>
                <label className="mcp-toggle">
                  <input type="checkbox" checked={settings.limits.hard_period_limits}
                    onChange={e => patch({ limits: { ...settings.limits, hard_period_limits: e.target.checked } })} />
                  <span>Stop requests at daily and monthly limits (otherwise only warn)</span>
                </label>
                <div className="field-hint">Per-run limits always stop the run. A request already in flight can overshoot a limit.</div>
              </div>
            </>
          )}
          {message && <div className="field-hint" role="status">{message}</div>}
        </div>
        <div className="modal-ft">
          <button className="btn secondary" onClick={onClose}>Close</button>
          <button className="btn primary" onClick={save} disabled={!settings}>Save prices and limits</button>
        </div>
      </div>
    </div>
  )
}
