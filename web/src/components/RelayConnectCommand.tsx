import { useEffect, useState } from 'react'
import { api, type CLIRelease, type RelayOffer } from '../lib/api'
import { Command } from './SetupCommand'
import { Button } from './ui'

export function supportsWorkbenchRelay(version?: string) {
  const match = /^v(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/.exec(version || '')
  if (!match) return false
  const [, major, minor, patch, rc] = match
  return Number(major) > 0 || Number(minor) > 1 || (Number(minor) === 1 && (Number(patch) > 0 || !rc || Number(rc) >= 2))
}

function quote(value: string) { return `'${value.replaceAll("'", "'\\''")}'` }

export function RelayConnectCommand({ release, refresh = false }: { release?: CLIRelease | null; refresh?: boolean }) {
  const [offer, setOffer] = useState<RelayOffer | null>(null)
  const [loading, setLoading] = useState(true)
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let active = true
    setLoading(true)
    void (release === undefined ? api.cliRelease() : Promise.resolve(release)).then(async (available) => {
      if (available?.status !== 'available' || !supportsWorkbenchRelay(available.version)) return null
      return await api.relayOffer()
    }).then((value) => { if (active) setOffer(value) }).catch(() => {
      if (active) setOffer(null)
    }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [release, revision])
  useEffect(() => {
    if (!offer?.expires_at) return
    const delay = Date.parse(offer.expires_at) - Date.now() - 30_000
    if (!Number.isFinite(delay) || delay <= 0) return
    const timer = setTimeout(() => { setOffer(null); setRevision((value) => value + 1) }, delay)
    return () => clearTimeout(timer)
  }, [offer])
  // This build only pairs through the workbench relay; never show a command that lets the CLI pick a public relay.
  const legacy = release !== undefined && !(release?.status === 'available' && supportsWorkbenchRelay(release.version))
  const expired = Boolean(offer?.available) && !(Date.parse(offer?.expires_at || '') > Date.now())
  const valid = offer?.available && offer.workbench?.startsWith('https://') && /^[\w-]{32,128}$/.test(offer.token || '') && !expired
  let command = ''
  if (valid) {
    command = `~/.local/bin/herdrx connect${refresh ? ' --refresh-endpoint' : ''} --plain --workbench ${quote(offer.workbench!)} --relay-token ${quote(offer.token!)}`
    if (offer.address && /^[\dA-Fa-f:.]+$/.test(offer.address)) command += ` --relay-address ${quote(offer.address)}`
  }
  const problem = legacy ? '可下载的 CLI 版本不支持工作台中继。请在远程主机安装本工作台配套的自有中继版 herdrx 后重试。'
    : expired ? '工作台中继授权已过期，请重新获取。'
    : offer === null ? '无法获取工作台中继授权，请检查网络与登录状态后重试。'
    : '工作台中继不可用：需要公网 HTTPS 地址、反向代理支持 /derp 且自检通过。修复工作台后重试；本版本不使用公共中继。'
  if (loading) return <p className="field-hint" role="status">正在准备连接命令…</p>
  if (!valid) return <div className="notice notice-info" role="status"><p>{problem}</p><Button type="button" className="button-ghost" onClick={() => setRevision((value) => value + 1)}>重新获取</Button></div>
  return <>
    <Command title={refresh ? '生成端点更新包' : '生成绑定凭据'} value={command}/>
    <p className="field-hint">使用工作台中继，连不通时报错终止，不改用公共中继。请使用本工作台配套的 CLI 执行；命令包含临时授权，请勿分享。</p>
  </>
}
