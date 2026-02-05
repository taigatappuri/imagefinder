import { useEffect, useMemo, useState } from 'react'

const API_BASE = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'

type Job = {
  id: string
  status: string
  progress: number
  error_message?: string | null
}

type SearchItem = {
  id: string
  base_url: string
  created_time?: string | null
  location?: string | null
  score: number
}

type PickerPollingConfig = {
  pollInterval?: string
  timeoutIn?: string
}

type PickerSessionResponse = {
  session_id: string
  picker_uri: string
  polling_config?: PickerPollingConfig | null
}

type PickerImportStatus = {
  status: string
  total: number
  processed: number
  imported: number
  failed: number
  remaining?: number
  warning?: string
  error?: string
}

type AppConfig = {
  google_photos_mode?: string
  indexing_available?: boolean
  picker_available?: boolean
}

export default function App() {
  const [authenticated, setAuthenticated] = useState(false)
  const [checkingAuth, setCheckingAuth] = useState(true)
  const [job, setJob] = useState<Job | null>(null)
  const [query, setQuery] = useState('')
  const [limit, setLimit] = useState(12)
  const [fromDate, setFromDate] = useState('')
  const [toDate, setToDate] = useState('')
  const [location, setLocation] = useState('')
  const [results, setResults] = useState<SearchItem[]>([])
  const [thumbnailUrls, setThumbnailUrls] = useState<Record<string, string>>({})
  const [statusMessage, setStatusMessage] = useState('')
  const [loadingSearch, setLoadingSearch] = useState(false)
  const [showScore, setShowScore] = useState(true)
  const [pickerSession, setPickerSession] = useState<PickerSessionResponse | null>(null)
  const [pickerStatus, setPickerStatus] = useState('')
  const [importingPicker, setImportingPicker] = useState(false)
  const [pickerImportId, setPickerImportId] = useState<string | null>(null)
  const [pickerImportStatus, setPickerImportStatus] = useState<PickerImportStatus | null>(null)
  const [appConfig, setAppConfig] = useState<AppConfig | null>(null)

  const hasResults = results.length > 0
  const isPickerMode =
    appConfig?.google_photos_mode === 'picker' || Boolean(appConfig?.picker_available)

  const jobLabel = useMemo(() => {
    if (!job) return 'まだジョブはありません'
    if (job.status === 'queued') return '待機中'
    if (job.status === 'running') return `実行中 ${job.progress}%`
    if (job.status === 'done') return '完了'
    if (job.status === 'failed') return '失敗'
    return job.status
  }, [job])

  useEffect(() => {
    const fetchAuth = async () => {
      try {
        const res = await fetch(`${API_BASE}/auth/me`, { credentials: 'include' })
        const data = await res.json()
        setAuthenticated(Boolean(data.authenticated))
      } catch {
        setAuthenticated(false)
      } finally {
        setCheckingAuth(false)
      }
    }
    fetchAuth()
  }, [])

  useEffect(() => {
    if (!authenticated || isPickerMode) return
    const timer = setInterval(async () => {
      try {
        const res = await fetch(`${API_BASE}/index/status`, { credentials: 'include' })
        const data = await res.json()
        setJob(data.job ?? null)
      } catch {
        setJob(null)
      }
    }, 3000)
    return () => clearInterval(timer)
  }, [authenticated, isPickerMode])

  useEffect(() => {
    const fetchConfig = async () => {
      try {
        const res = await fetch(`${API_BASE}/config`)
        if (!res.ok) return
        const data = await res.json()
        setAppConfig(data)
      } catch {
        return
      }
    }
    fetchConfig()
  }, [])

  useEffect(() => {
    if (results.length === 0) return
    const activeIds = new Set(results.map((item) => item.id))
    setThumbnailUrls((prev) => {
      const next: Record<string, string> = {}
      for (const [id, url] of Object.entries(prev)) {
        if (activeIds.has(id)) {
          next[id] = url
        } else {
          URL.revokeObjectURL(url)
        }
      }
      return next
    })
  }, [results])

  useEffect(() => {
    if (!authenticated || results.length === 0) return
    let cancelled = false
    const controller = new AbortController()

    const loadThumbnail = async (item: SearchItem) => {
      if (thumbnailUrls[item.id]) return
      try {
        const res = await fetch(`${API_BASE}/photos/${item.id}/thumbnail`, {
          credentials: 'include',
          signal: controller.signal
        })
        if (!res.ok) return
        const blob = await res.blob()
        if (cancelled) return
        const url = URL.createObjectURL(blob)
        setThumbnailUrls((prev) => ({ ...prev, [item.id]: url }))
      } catch {
        return
      }
    }

    results.forEach((item) => {
      loadThumbnail(item)
    })

    return () => {
      cancelled = true
      controller.abort()
    }
  }, [results, authenticated, thumbnailUrls])

  const startLogin = () => {
    window.location.href = `${API_BASE}/auth/google`
  }

  const startIndex = async () => {
    if (isPickerMode) {
      setStatusMessage('Picker モードではインデックス更新は利用できません。下の「写真を選択する」から取り込みしてください。')
      return
    }
    setStatusMessage('インデックス作成を開始します')
    try {
      const res = await fetch(`${API_BASE}/index/update`, {
        method: 'POST',
        credentials: 'include'
      })
      const data = await res.json()
      if (!res.ok) {
        setStatusMessage(data.error || 'インデックス開始に失敗しました')
        return
      }
      setStatusMessage('ジョブを作成しました')
    } catch {
      setStatusMessage('インデックス開始に失敗しました')
    }
  }

  const parseDurationToMs = (value?: string | null) => {
    if (!value) return 3000
    const trimmed = value.trim()
    if (trimmed.endsWith('ms')) {
      const num = Number(trimmed.replace('ms', ''))
      return Number.isNaN(num) ? 3000 : num
    }
    if (trimmed.endsWith('s')) {
      const num = Number(trimmed.replace('s', ''))
      return Number.isNaN(num) ? 3000 : num * 1000
    }
    const num = Number(trimmed)
    return Number.isNaN(num) ? 3000 : num
  }

  const startPicker = async () => {
    setPickerStatus('Picker セッションを作成しています...')
    try {
      const res = await fetch(`${API_BASE}/picker/session`, {
        method: 'POST',
        credentials: 'include'
      })
      const data = await res.json()
      if (!res.ok) {
        setPickerStatus(data.error || 'Picker セッションの作成に失敗しました')
        return
      }
      const session: PickerSessionResponse = {
        session_id: data.session_id,
        picker_uri: data.picker_uri,
        polling_config: data.polling_config
      }
      setPickerSession(session)
      setPickerStatus('写真選択画面を開きました')
      window.open(session.picker_uri, '_blank', 'noopener,noreferrer')
    } catch {
      setPickerStatus('Picker セッションの作成に失敗しました')
    }
  }

  const importPickerSession = async (sessionId: string) => {
    if (importingPicker) return
    setImportingPicker(true)
    setPickerStatus('選択された写真を取り込んでいます...')
    try {
      const res = await fetch(`${API_BASE}/picker/import`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ session_id: sessionId })
      })
      const data = await res.json()
      if (!res.ok) {
        setPickerStatus(data.error || '取り込みに失敗しました')
        setImportingPicker(false)
        return
      }
      const importId = data.import_id || sessionId
      const total = data.total ?? 0
      setPickerImportId(importId)
      setPickerImportStatus({
        status: data.status || 'running',
        total,
        processed: 0,
        imported: 0,
        failed: 0,
        remaining: total
      })
    } catch {
      setPickerStatus('取り込みに失敗しました')
      setImportingPicker(false)
    } finally {
      return
    }
  }

  const runSearch = async () => {
    setLoadingSearch(true)
    setStatusMessage('検索中...')
    try {
      const res = await fetch(`${API_BASE}/search`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          query,
          limit,
          filters: {
            from: fromDate || undefined,
            to: toDate || undefined,
            location: location || undefined
          }
        })
      })
      const data = await res.json()
      if (!res.ok) {
        setStatusMessage(data.error || '検索に失敗しました')
        return
      }
      setResults(data.items || [])
      setStatusMessage(`${data.items?.length ?? 0} 件の結果`)
    } catch {
      setStatusMessage('検索に失敗しました')
    } finally {
      setLoadingSearch(false)
    }
  }

  useEffect(() => {
    if (!pickerSession || !authenticated) return
    const intervalMs = parseDurationToMs(pickerSession.polling_config?.pollInterval)
    let stopped = false

    const poll = async () => {
      if (stopped) return
      try {
        const res = await fetch(`${API_BASE}/picker/session/${pickerSession.session_id}`, {
          credentials: 'include'
        })
        const data = await res.json()
        if (!res.ok) {
          setPickerStatus(data.error || 'Picker セッションの確認に失敗しました')
          return
        }
        if (data.media_items_set) {
          stopped = true
          await importPickerSession(pickerSession.session_id)
        }
      } catch {
        setPickerStatus('Picker セッションの確認に失敗しました')
      }
    }

    const timer = setInterval(poll, intervalMs)
    poll()
    return () => {
      stopped = true
      clearInterval(timer)
    }
  }, [pickerSession, authenticated])

  useEffect(() => {
    if (!pickerImportId || !authenticated) return
    let stopped = false

    const poll = async () => {
      if (stopped) return
      try {
        const res = await fetch(`${API_BASE}/picker/import/status/${pickerImportId}`, {
          credentials: 'include'
        })
        const data = await res.json()
        if (!res.ok) {
          setPickerStatus(data.error || '取り込み状況の取得に失敗しました')
          setImportingPicker(false)
          setPickerImportId(null)
          return
        }
        setPickerImportStatus(data)
        if (data.status === 'done') {
          const warning = data.warning ? ` / ${data.warning}` : ''
          if (data.failed > 0) {
            setPickerStatus(`取り込み完了: ${data.imported ?? 0} 件 (失敗 ${data.failed ?? 0} 件)${warning}`)
          } else {
            setPickerStatus(`取り込み完了: ${data.imported ?? 0} 件${warning}`)
          }
          setPickerSession(null)
          setImportingPicker(false)
          setPickerImportId(null)
        }
        if (data.status === 'failed') {
          setPickerStatus(data.error || '取り込みに失敗しました')
          setImportingPicker(false)
          setPickerImportId(null)
        }
      } catch {
        setPickerStatus('取り込み状況の取得に失敗しました')
        setImportingPicker(false)
        setPickerImportId(null)
      }
    }

    const timer = setInterval(poll, 2000)
    poll()
    return () => {
      stopped = true
      clearInterval(timer)
    }
  }, [pickerImportId, authenticated])

  return (
    <div className="page">
      <header className="hero">
        <div className="hero-text">
          <p className="eyebrow">Google Photos 連携・自然言語検索</p>
          <h1>ImageFinder</h1>
          <p className="lead">
            自然言語で写真を探し、シーンや記憶にすばやくアクセスするための検索体験。
          </p>
        </div>
        <div className="hero-card">
          <h2>はじめに</h2>
          <p>ログインしてインデックスを作成し、検索を開始します。</p>
          {checkingAuth ? (
            <div className="status">認証状態を確認中...</div>
          ) : authenticated ? (
            <div className="status ok">ログイン済み</div>
          ) : (
            <button className="primary" onClick={startLogin}>
              Google でログイン
            </button>
          )}
        </div>
      </header>

      <section className="panel">
        <div className="panel-header">
          <h2>インデックス更新</h2>
          <p>Google Photos の情報を取得し、検索用に準備します。</p>
        </div>
        <div className="panel-body">
          {isPickerMode ? (
            <div className="status">
              Picker モードのため全件インデックス更新は利用できません。下の「写真を選択する」から取り込みしてください。
            </div>
          ) : (
            <>
              <button className="secondary" onClick={startIndex} disabled={!authenticated}>
                インデックスを開始
              </button>
              <div className="job-status">
                <span>進捗:</span>
                <strong>{jobLabel}</strong>
                {job?.error_message ? <span className="error">{job.error_message}</span> : null}
              </div>
            </>
          )}
        </div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Google Photos から選択</h2>
          <p>Picker で写真を選び、そのままインデックスに追加します。</p>
        </div>
        <div className="panel-body">
          <button className="secondary" onClick={startPicker} disabled={!authenticated || importingPicker}>
            {importingPicker ? '取り込み中...' : '写真を選択する'}
          </button>
          {pickerStatus ? <div className="status">{pickerStatus}</div> : null}
          {pickerImportStatus ? (
            <div className="progress">
              <progress
                value={pickerImportStatus.processed}
                max={pickerImportStatus.total > 0 ? pickerImportStatus.total : 1}
              />
              <div className="progress-meta">
                {pickerImportStatus.total > 0
                  ? `残り ${
                      pickerImportStatus.remaining ?? Math.max(pickerImportStatus.total - pickerImportStatus.processed, 0)
                    } / 全 ${pickerImportStatus.total}`
                  : '取り込み準備中...'}
              </div>
            </div>
          ) : null}
        </div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>検索</h2>
          <p>例: 「海辺で夕焼け」「料理中の写真」「犬と走っている写真」</p>
        </div>
        <div className="panel-body search-form">
          <div className="field wide">
            <label>検索クエリ</label>
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="自然言語で検索"
            />
          </div>
          <div className="field">
            <label>開始日</label>
            <input type="date" value={fromDate} onChange={(event) => setFromDate(event.target.value)} />
          </div>
          <div className="field">
            <label>終了日</label>
            <input type="date" value={toDate} onChange={(event) => setToDate(event.target.value)} />
          </div>
          <div className="field">
            <label>場所</label>
            <input value={location} onChange={(event) => setLocation(event.target.value)} placeholder="東京 など" />
          </div>
          <div className="field">
            <label>件数</label>
            <input
              type="number"
              min={1}
              max={30}
              value={limit}
              onChange={(event) => setLimit(Number(event.target.value))}
            />
          </div>
          <div className="field toggle">
            <label>スコア表示</label>
            <button
              type="button"
              className={showScore ? 'toggle-button active' : 'toggle-button'}
              onClick={() => setShowScore((prev) => !prev)}
            >
              {showScore ? '表示中' : '非表示'}
            </button>
          </div>
          <button className="primary" onClick={runSearch} disabled={!authenticated || loadingSearch}>
            {loadingSearch ? '検索中...' : '検索する'}
          </button>
        </div>
      </section>

      <section className="panel results">
        <div className="panel-header">
          <h2>検索結果</h2>
          <p>類似度が高い順に表示されます。</p>
        </div>
        <div className="panel-body">
          {statusMessage && <div className="status">{statusMessage}</div>}
          {!hasResults ? (
            <div className="empty">結果はまだありません</div>
          ) : (
            <div className="grid">
              {results.map((item) => (
                <article key={item.id} className="card">
                  <div className="thumb">
                    {thumbnailUrls[item.id] ? (
                      <img src={thumbnailUrls[item.id]} alt="search result" loading="lazy" />
                    ) : (
                      <div className="thumb-placeholder">読み込み中...</div>
                    )}
                  </div>
                  <div className="meta">
                    {showScore ? (
                      <div>
                        <span className="label">スコア</span>
                        <strong>{item.score.toFixed(3)}</strong>
                      </div>
                    ) : null}
                    <div>
                      <span className="label">日時</span>
                      <span>{item.created_time ? item.created_time.slice(0, 10) : '-'}</span>
                    </div>
                    <div>
                      <span className="label">場所</span>
                      <span>{item.location || '-'}</span>
                    </div>
                    <a className="link" href={item.base_url} target="_blank" rel="noreferrer">
                      Google Photos を開く
                    </a>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      </section>

      <footer className="footer">
        <span>ImageFinder MVP</span>
      </footer>
    </div>
  )
}
