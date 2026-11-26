import { useState, useEffect } from 'react'
import {
  GetSandboxStatus, ExportDiagnostics,
} from '../../../wailsjs/go/main/App'
import type { Dispatch, SetStateAction } from 'react'
import { ModelSettingsPanel } from '../ModelSettingsPanel'
import { DataSettings } from './DataSettings'
import type { Cfg } from '../../types'

interface Props {
  cfg: Cfg
  setCfg: Dispatch<SetStateAction<Cfg>>
  models: string[]
  setModels: (models: string[]) => void
  whisperReady: boolean
  appVersion: string
  onImported: () => void
}

export function GeneralTab({ cfg, setCfg, models, setModels, whisperReady, appVersion, onImported }: Props) {
  const [diagMsg, setDiagMsg] = useState('')
  // Isolation actually applied to shell commands under the saved settings.
  const [sandboxStatus, setSandboxStatus] = useState<{ backend: string; isolated: boolean; network_blocked: boolean; detail: string } | null>(null)
  useEffect(() => {
    GetSandboxStatus().then(setSandboxStatus).catch(() => setSandboxStatus(null))
  }, [])

  return (
    <div className="modal-body">
      <div className="field">
        <label htmlFor="provider">Provider</label>
        <select id="provider" value={cfg.provider} onChange={e => setCfg(c => ({ ...c, provider: e.target.value }))}>
          <optgroup label="── Free · No API Key ──">
            <option value="ollama">Ollama (local)</option>
            <option value="llm7">LLM7.io — DeepSeek, GPT-4o, Gemini</option>
            <option value="zai">Z AI (Zhipu) — GLM-4.7-Flash</option>
          </optgroup>
          <optgroup label="── Free Tier · API Token Required ──">
            <option value="github">GitHub Models — GPT-5, Llama 4, Grok 3</option>
            <option value="groq">Groq — Llama 4, Qwen3, GPT-OSS</option>
            <option value="gemini">Google Gemini (AI Studio)</option>
            <option value="cerebras">Cerebras — GPT-OSS-120B (very fast)</option>
            <option value="openrouter">OpenRouter — 10+ free models</option>
            <option value="mistral">Mistral AI</option>
            <option value="cohere">Cohere</option>
            <option value="together">Together AI</option>
            <option value="huggingface">Hugging Face</option>
            <option value="sambanova">SambaNova Cloud</option>
            <option value="nvidia">NVIDIA NIM</option>
          </optgroup>
          <optgroup label="── Paid / Custom ──">
            <option value="openai">OpenAI-compatible</option>
          </optgroup>
        </select>
      </div>

      {(cfg.provider === 'ollama' || cfg.provider === 'openai') && (
        <div className="field">
          <label htmlFor="base-url">Base URL</label>
          <input id="base-url" value={cfg.base_url} onChange={e => setCfg(c => ({ ...c, base_url: e.target.value }))}
            placeholder="http://localhost:11434" />
        </div>
      )}

      {!['ollama', 'llm7', 'zai'].includes(cfg.provider) && (
        <div className="field">
          <label>API Key{cfg.provider === 'github' ? ' (GitHub PAT with models:read)' : ''}</label>
          <input type="password" value={cfg.api_key}
            onChange={e => setCfg(c => ({ ...c, api_key: e.target.value }))}
            placeholder={cfg.provider === 'github' ? 'github_pat_…' : 'sk-…'} />
        </div>
      )}

      <div className="field">
        <span className="field-label">Default Model</span>
        {models.length > 0
          ? <select value={cfg.model} onChange={e => setCfg(c => ({ ...c, model: e.target.value }))}>
              {(() => {
                const freeModels = models.filter(m => m.endsWith(':free'))
                const paidModels = models.filter(m => !m.endsWith(':free'))
                if (freeModels.length > 0 && paidModels.length > 0) {
                  return (
                    <>
                      <optgroup label="── Free ──">
                        {freeModels.map(m => <option key={m} value={m}>{m}</option>)}
                      </optgroup>
                      <optgroup label="── Paid ──">
                        {paidModels.map(m => <option key={m} value={m}>{m}</option>)}
                      </optgroup>
                    </>
                  )
                }
                return models.map(m => <option key={m} value={m}>{m}</option>)
              })()}
            </select>
          : <input value={cfg.model} onChange={e => setCfg(c => ({ ...c, model: e.target.value }))} placeholder="gemma2:27b" />
        }
      </div>

      <div className="field">
        <label htmlFor="context-limit-tokens">Context Limit (tokens)</label>
        <input id="context-limit-tokens"
          type="number" min={1024} max={131072} step={1024}
          value={cfg.context_limit || 8192}
          onChange={e => setCfg(c => ({ ...c, context_limit: Number(e.target.value) }))}
        />
        <div className="field-hint">Approximate size of what is sent to the model (default: 8192). History is never deleted.</div>
      </div>

      <div className="field">
        <span className="field-label">Execution limits per run (0 = default)</span>
        <div className="usage-grid">
          {([['max_rounds', 'Model rounds (50)'], ['max_parallel_tools', 'Tools at once (4)'], ['tool_timeout_seconds', 'Seconds per tool (360)']] as const).map(([k, label]) => (
            <label key={k}>{label}
              <input type="number" min={0} value={cfg[k] || 0}
                onChange={e => setCfg(c => ({ ...c, [k]: Math.max(0, Number(e.target.value)) }))} />
            </label>
          ))}
        </div>
      </div>

      <div className="field">
        <span className="field-label">Diagnostics</span>
        <button className="btn secondary" onClick={() => ExportDiagnostics().then(r => setDiagMsg(r === 'ok' ? '✓ Diagnostics saved' : r ? '✗ ' + r : ''))}>Export diagnostics…</button>
        <div className="field-hint">Versions, settings without keys or addresses, MCP state and the app log. No conversation content. Nothing is sent anywhere.</div>
        {diagMsg && <div role="status" className="field-hint">{diagMsg}</div>}
      </div>

      <div className="field">
        <label htmlFor="context-strategy">Older turns</label>
        <select id="context-strategy" value={cfg.context_strategy || 'sliding'}
          onChange={e => setCfg(c => ({ ...c, context_strategy: e.target.value }))}>
          <option value="sliding">Sliding window — leave them out</option>
          <option value="hybrid">Hybrid — keep what fits, summarise the rest</option>
          <option value="summary">Summary — keep two turns, summarise the rest</option>
        </select>
        <div className="field-hint">Summaries are written by the model, cost extra requests and can be wrong.</div>
      </div>

      <div className="field">
        <label className="mcp-toggle">
          <input
            type="checkbox"
            checked={cfg.notifications !== false}
            onChange={e => setCfg(c => ({ ...c, notifications: e.target.checked }))}
          />
          <span>Desktop notifications</span>
        </label>
        <div className="field-hint">Alert me when a long task finishes, a permission is needed, or an error occurs — only while the window is in the background.</div>
      </div>

      <ModelSettingsPanel provider={cfg.provider} model={cfg.model} onModels={setModels} />

      <div className="field">
        <label htmlFor="tool-calling">Tool calling</label>
        <select id="tool-calling" value={cfg.tool_calling || 'auto'}
          onChange={e => setCfg(c => ({ ...c, tool_calling: e.target.value }))}>
          <option value="auto">Automatic — native when the model supports it</option>
          <option value="text">Text protocol</option>
        </select>
        <div className="field-hint">Choose the text protocol if a model claims to support tools but uses them badly.</div>
      </div>

      <div className="field">
        <label htmlFor="sandbox-mode">Shell sandbox</label>
        <select id="sandbox-mode" value={cfg.sandbox_mode || 'auto'}
          onChange={e => setCfg(c => ({ ...c, sandbox_mode: e.target.value }))}>
          <option value="auto">Automatic — isolate when the system supports it</option>
          <option value="strict">Strict — refuse to run without isolation</option>
          <option value="off">Off — commands can reach the whole computer</option>
        </select>
        <label className="mcp-toggle">
          <input type="checkbox" checked={!!cfg.sandbox_network}
            onChange={e => setCfg(c => ({ ...c, sandbox_network: e.target.checked }))} />
          <span>Allow network access inside the sandbox</span>
        </label>
        <div className="field-hint" role="status">
          {sandboxStatus === null ? 'Checking isolation…'
            : sandboxStatus.isolated
              ? `Saved settings: isolated with ${sandboxStatus.backend}, network ${sandboxStatus.network_blocked ? 'blocked' : 'allowed'}.`
              : `Saved settings: NOT isolated (${sandboxStatus.detail}).`}
        </div>
      </div>

      <div className="field">
        <label htmlFor="theme">Theme</label>
        <select id="theme"
          value={cfg.theme || 'dark'}
          onChange={e => setCfg(c => ({ ...c, theme: e.target.value }))}
        >
          <option value="dark">Dark</option>
          <option value="light">Light</option>
          <option value="system">Follow OS</option>
        </select>
        <div className="field-hint">Applies immediately. "Follow OS" tracks your system's light/dark preference.</div>
      </div>


      <div className="field">
        <label htmlFor="web-search-provider">Web Search Provider</label>
        <select id="web-search-provider"
          value={cfg.search_provider || 'duckduckgo'}
          onChange={e => setCfg(c => ({ ...c, search_provider: e.target.value }))}
        >
          <option value="duckduckgo">DuckDuckGo — no API key required</option>
          <option value="brave">Brave Search — requires API key</option>
          <option value="serpapi">SerpAPI (Google) — requires API key</option>
        </select>
        <div className="field-hint">Powers the <code>search_web</code> tool. DuckDuckGo works out of the box; Brave/SerpAPI return higher-quality results with an API key.</div>
      </div>

      {(cfg.search_provider === 'brave' || cfg.search_provider === 'serpapi') && (
        <div className="field">
          <label>{cfg.search_provider === 'brave' ? 'Brave Search' : 'SerpAPI'} API Key</label>
          <input
            type="password"
            value={cfg.search_api_key || ''}
            onChange={e => setCfg(c => ({ ...c, search_api_key: e.target.value }))}
            placeholder={cfg.search_provider === 'brave' ? 'BSA…' : 'serpapi key'}
          />
        </div>
      )}

      <div className="field">
        <label htmlFor="system-prompt-optional-prefix">System Prompt (optional prefix)</label>
        <textarea id="system-prompt-optional-prefix"
          rows={4}
          value={cfg.system_prompt || ''}
          onChange={e => setCfg(c => ({ ...c, system_prompt: e.target.value }))}
          placeholder="Add custom instructions that will be prepended to the agent's system prompt…"
          style={{ resize: 'vertical', fontFamily: 'monospace', fontSize: 12 }}
        />
      </div>

      <div className="field">
        <label htmlFor="whisper-model-speech-to-text">Whisper Model (speech-to-text)</label>
        <select id="whisper-model-speech-to-text" value={cfg.whisper_model || 'base'} onChange={e => setCfg(c => ({ ...c, whisper_model: e.target.value }))}>
          <option value="tiny">tiny — fastest, lower accuracy (~39 MB)</option>
          <option value="base">base — fast, decent accuracy (~74 MB)</option>
          <option value="small">small — balanced (~244 MB)</option>
          <option value="medium">medium — high accuracy (~769 MB)</option>
          <option value="large">large — best accuracy (~1.5 GB)</option>
        </select>
        {!whisperReady && (
          <div className="field-hint">
            whisper not found — install with: <code>pip install openai-whisper</code>
          </div>
        )}
      </div>

      <DataSettings cfg={cfg} setCfg={setCfg} onImported={onImported} />

      {appVersion && (
        <div style={{ fontSize: 11, color: 'var(--text3)', marginTop: 8 }}>
          Version: {appVersion}
        </div>
      )}
    </div>
  )
}
