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
  const [statusMessage, setStatusMessage] = useState('')
  const [loadingSearch, setLoadingSearch] = useState(false)
  const [showScore, setShowScore] = useState(true)

  const hasResults = results.length > 0

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
    if (!authenticated) return
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
  }, [authenticated])

  const startLogin = () => {
    window.location.href = `${API_BASE}/auth/google`
  }

  const startIndex = async () => {
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
          <button className="secondary" onClick={startIndex} disabled={!authenticated}>
            インデックスを開始
          </button>
          <div className="job-status">
            <span>進捗:</span>
            <strong>{jobLabel}</strong>
            {job?.error_message ? <span className="error">{job.error_message}</span> : null}
          </div>
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
                  {(() => {
                    const isGoogle = item.base_url.includes('googleusercontent') || item.base_url.includes('photoslibrary')
                    const thumbUrl = isGoogle ? `${item.base_url}=w600-h600` : item.base_url
                    return (
                      <>
                        <div className="thumb">
                          <img src={thumbUrl} alt="search result" loading="lazy" />
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
                      </>
                    )
                  })()}
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
