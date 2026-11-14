import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { api } from '../lib/api'
import { RelayConnectCommand } from './RelayConnectCommand'

vi.mock('../lib/api', () => ({ api: { cliRelease: vi.fn(), relayOffer: vi.fn() } }))
afterEach(() => vi.resetAllMocks())

const valid = { available: true, workbench: 'https://workbench.example.com', token: 'a'.repeat(43), expires_at: new Date(Date.now() + 1_200_000).toISOString() }

it('refuses a CLI that predates workbench relay support without offering a plain command', async () => {
  render(<RelayConnectCommand release={{ status: 'available', version: 'v0.1.0-rc.1' }}/>)
  expect(await screen.findByText(/自有中继版/)).toBeInTheDocument()
  expect(screen.queryByLabelText('生成绑定凭据')).toBeNull()
  expect(api.relayOffer).not.toHaveBeenCalled()
})

it('offers a scoped relay command for endpoint migration after release discovery', async () => {
  vi.mocked(api.cliRelease).mockResolvedValue({ status: 'available', version: 'v0.1.0-rc.2' })
  vi.mocked(api.relayOffer).mockResolvedValue({ ...valid, address: '203.0.113.20' })
  render(<RelayConnectCommand refresh/>)
  expect(await screen.findByLabelText('生成端点更新包')).toHaveTextContent(`~/.local/bin/herdrx connect --refresh-endpoint --plain --workbench 'https://workbench.example.com' --relay-token '${'a'.repeat(43)}' --relay-address '203.0.113.20'`)
})

it.each([
  ['unavailable', '工作台中继不可用'],
  ['failed', '无法获取工作台中继授权'],
  ['expired', '已过期'],
])('shows no command and retries when relay discovery is %s', async (state, message) => {
  if (state === 'failed') vi.mocked(api.relayOffer).mockRejectedValueOnce(new Error('offline'))
  else vi.mocked(api.relayOffer).mockResolvedValueOnce(state === 'unavailable' ? { available: false } : { ...valid, expires_at: new Date(0).toISOString() })
  vi.mocked(api.relayOffer).mockResolvedValueOnce(valid)
  render(<RelayConnectCommand release={{ status: 'available', version: 'v0.1.0-rc.2' }}/>)
  expect(await screen.findByText(new RegExp(message))).toBeInTheDocument()
  expect(screen.queryByLabelText('生成绑定凭据')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '重新获取' }))
  expect(await screen.findByLabelText('生成绑定凭据')).toHaveTextContent(`~/.local/bin/herdrx connect --plain --workbench 'https://workbench.example.com' --relay-token '${'a'.repeat(43)}'`)
  expect(api.relayOffer).toHaveBeenCalledTimes(2)
})
