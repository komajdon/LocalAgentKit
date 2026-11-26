// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const report = vi.fn()
const save = vi.fn().mockResolvedValue('')
vi.mock('../../wailsjs/go/main/App', () => ({
  GetUsageReport: (f: unknown) => report(f),
  GetUsageSettings: () => Promise.resolve({ currency: 'USD', prices: [], limits: { warn_percent: 80, hard_period_limits: false, run_tokens: 0, run_requests: 0, run_cost: 0, daily_tokens: 0, daily_cost: 0, monthly_tokens: 0, monthly_cost: 0 } }),
  SaveUsageSettings: (s: unknown) => save(s),
}))

const { UsageDashboard, costLabel } = await import('./UsageDashboard')

const entry = { day: '2026-11-26', provider: 'ollama', model: 'm', conversation_id: 'c1', requests: 2, estimated_requests: 1, input: 1200, output: 30, cached: 0, cost: 0, unpriced_requests: 2 }
report.mockResolvedValue({ entries: [entry], total: entry })
afterEach(cleanup)

describe('usage dashboard', () => {
  it('never presents a missing price as zero cost', () => {
    expect(costLabel({ cost: 0, requests: 2, unpriced_requests: 2 }, 'USD')).toBe('no price set')
    expect(costLabel({ cost: 1, requests: 3, unpriced_requests: 1 }, 'USD')).toBe('≈ 1.0000 USD + 1 unpriced')
  })

  it('filters the report and saves limits', async () => {
    render(<UsageDashboard conversations={[{ id: 'c1', title: 'My chat' }]} onClose={() => {}} />)
    await screen.findAllByText('My chat')
    expect(screen.getByText('~1,200')).toBeTruthy()

    fireEvent.change(screen.getByLabelText('Model'), { target: { value: 'm' } })
    await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ model: 'm' })))

    fireEvent.change(screen.getByLabelText('Tokens per day'), { target: { value: '500' } })
    fireEvent.click(screen.getByText('Save prices and limits'))
    await waitFor(() => expect(save).toHaveBeenCalledWith(expect.objectContaining({ limits: expect.objectContaining({ daily_tokens: 500 }) })))
  })

  it('closes on Escape', () => {
    const onClose = vi.fn()
    render(<UsageDashboard conversations={[]} onClose={onClose} />)
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).toHaveBeenCalled()
  })
})
