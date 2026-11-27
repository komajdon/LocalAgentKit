import { Icon, toolIcon } from '../Icon'
import { failed, toolName, type Step } from './blocks'

/** Compact rows for a run of tool calls; each opens to show its details. */
export function ToolActivity({ steps, running }: { steps: Step[]; running: boolean }) {
  return (
    <div className="activity">
      {steps.map(step => {
        const state = step.result === undefined ? (running ? 'running' : 'none') : failed(step.result) ? 'bad' : 'ok'
        return (
          <details key={step.id} className="tool-step">
            <summary>
              <span className="chevron"><Icon name="chevron" size={13} /></span>
              <Icon name={toolIcon(step.tool)} size={15} />
              <span className="tool-name">{toolName(step.tool)}</span>
              <span className="tool-args">{step.args}</span>
              <span className={`tool-state ${state}`} role="img"
                aria-label={{ ok: 'Succeeded', bad: 'Failed', running: 'Running', none: 'No result' }[state]}>
                {state === 'ok' && <Icon name="check" size={14} />}
                {state === 'bad' && <Icon name="x" size={14} />}
                {state === 'running' && <span className="thinking-dots"><span /><span /><span /></span>}
              </span>
            </summary>
            <div className="tool-detail">
              {step.args && <><div className="tool-detail-label">{step.tool} arguments</div><pre>{step.args}</pre></>}
              {step.result !== undefined && <><div className="tool-detail-label">Result</div><pre>{step.result}</pre></>}
            </div>
          </details>
        )
      })}
    </div>
  )
}
