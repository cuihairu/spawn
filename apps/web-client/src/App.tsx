import { useCallback, useState } from 'react'
import { BrowserRouter, Routes, Route, Link } from 'react-router-dom'
import type { UserInfo } from './api/client'
import HomePage from './pages/HomePage'
import CommunityPage from './pages/CommunityPage'
import GuidesPage from './pages/GuidesPage'
import GuideDetailPage from './pages/GuideDetailPage'
import GuideEditorPage from './pages/GuideEditorPage'
import PostDetailPage from './pages/PostDetailPage'
import './App.css'

const AUTH_STORAGE_KEY = 'tappi.auth'

function App() {
  const [auth, setAuth] = useState<{ token: string; user: UserInfo } | null>(() => {
    try {
      const raw = localStorage.getItem(AUTH_STORAGE_KEY)
      if (!raw) return null
      const parsed = JSON.parse(raw) as { token: string; user: UserInfo }
      if (!parsed?.token || !parsed?.user?.id) return null
      return parsed
    } catch {
      localStorage.removeItem(AUTH_STORAGE_KEY)
      return null
    }
  })

  const handleAuthChange = useCallback((next: { token: string; user: UserInfo } | null) => {
    setAuth(next)
    if (!next) {
      localStorage.removeItem(AUTH_STORAGE_KEY)
      return
    }
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(next))
  }, [])

  return (
    <BrowserRouter>
      <div className="app">
        <nav className="main-nav">
          <div className="nav-container">
            <Link to="/" className="nav-brand">
              🎮 Tappi
            </Link>
            <div className="nav-links">
              <Link to="/" className="nav-link">
                首页
              </Link>
              <Link to="/community" className="nav-link">
                社区
              </Link>
              <Link to="/guides" className="nav-link">
                攻略
              </Link>
              {auth ? (
                <>
                  <span className="nav-user">👤 {auth.user.nickname || auth.user.username}</span>
                  <button type="button" className="nav-logout" onClick={() => handleAuthChange(null)}>
                    退出登录
                  </button>
                </>
              ) : null}
            </div>
          </div>
        </nav>

        <main className="main-content">
          <Routes>
            <Route path="/" element={<HomePage auth={auth} onAuthChange={handleAuthChange} />} />
            <Route path="/community" element={<CommunityPage token={auth?.token} />} />
            <Route
              path="/community/posts/:id"
              element={<PostDetailPage token={auth?.token} userId={auth?.user.id} />}
            />
            <Route
              path="/guides"
              element={<GuidesPage token={auth?.token} userId={auth?.user.id} />}
            />
            <Route
              path="/guides/:id"
              element={<GuideDetailPage token={auth?.token} userId={auth?.user.id} />}
            />
            <Route
              path="/guides/:id/edit"
              element={
                auth ? (
                  <GuideEditorPage token={auth.token} userId={auth.user.id} />
                ) : (
                  <div className="error-page">
                    <p>请先登录</p>
                    <Link to="/">返回首页</Link>
                  </div>
                )
              }
            />
            <Route
              path="/guides/new"
              element={
                auth ? (
                  <GuideEditorPage token={auth.token} userId={auth.user.id} />
                ) : (
                  <div className="error-page">
                    <p>请先登录</p>
                    <Link to="/">返回首页</Link>
                  </div>
                )
              }
            />
          </Routes>
        </main>

        <footer className="main-footer">
          <p>© 2024 Tappi - 游戏体验服务平台</p>
        </footer>
      </div>
    </BrowserRouter>
  )
}

export default App
