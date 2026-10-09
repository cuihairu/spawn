import { useCallback, useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Link } from 'react-router-dom'
import { fetchUnreadNotificationCount, type UserInfo } from './api/client'
import HomePage from './pages/HomePage'
import DiscoverPage from './pages/DiscoverPage'
import CommunityPage from './pages/CommunityPage'
import GuidesPage from './pages/GuidesPage'
import GuideDetailPage from './pages/GuideDetailPage'
import GuideEditorPage from './pages/GuideEditorPage'
import GamesPage from './pages/GamesPage'
import GameDetailPage from './pages/GameDetailPage'
import NotificationsPage from './pages/NotificationsPage'
import PostDetailPage from './pages/PostDetailPage'
import ProfilePage from './pages/ProfilePage'
import StatsPage from './pages/StatsPage'
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

  // 站内通知未读数：登录后拉取 + 30s 轮询驱动导航铃铛徽标；登出在回调里清零。
  const [unreadNotifications, setUnreadNotifications] = useState(0)
  const handleAuthChange = useCallback((next: { token: string; user: UserInfo } | null) => {
    setAuth(next)
    if (!next) {
      localStorage.removeItem(AUTH_STORAGE_KEY)
      setUnreadNotifications(0)
      return
    }
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(next))
  }, [])

  useEffect(() => {
    if (!auth) return
    let cancelled = false
    const refresh = () => {
      fetchUnreadNotificationCount(auth.token)
        .then((count) => {
          if (!cancelled) setUnreadNotifications(count)
        })
        .catch(() => {
          // 静默：徽标轮询失败不打扰用户
        })
    }
    const timer = window.setInterval(refresh, 30_000)
    queueMicrotask(refresh)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [auth])

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
              <Link to="/discover" className="nav-link">
                发现
              </Link>
              <Link to="/games" className="nav-link">
                游戏库
              </Link>
              <Link to="/community" className="nav-link">
                社区
              </Link>
              <Link to="/guides" className="nav-link">
                攻略
              </Link>
              {auth ? (
                <>
                  <Link to="/notifications" className="nav-bell" aria-label={`未读通知 ${unreadNotifications} 条`}>
                    🔔
                    {unreadNotifications > 0 ? (
                      <span className="nav-badge">{unreadNotifications > 99 ? '99+' : unreadNotifications}</span>
                    ) : null}
                  </Link>
                  <Link to="/stats" className="nav-link">
                    战绩
                  </Link>
                  <Link to="/profile" className="nav-link">
                    个人中心
                  </Link>
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
            <Route path="/discover" element={<DiscoverPage />} />
            <Route path="/games" element={<GamesPage />} />
            <Route path="/games/:id" element={<GameDetailPage />} />
            <Route
              path="/stats"
              element={
                auth ? (
                  <StatsPage userId={auth.user.id} nickname={auth.user.nickname} />
                ) : (
                  <div className="error-page">
                    <p>请先登录</p>
                    <Link to="/">返回首页</Link>
                  </div>
                )
              }
            />
            <Route
              path="/profile"
              element={
                auth ? (
                  <ProfilePage token={auth.token} userId={auth.user.id} nickname={auth.user.nickname} />
                ) : (
                  <div className="error-page">
                    <p>请先登录</p>
                    <Link to="/">返回首页</Link>
                  </div>
                )
              }
            />
            <Route
              path="/notifications"
              element={
                auth ? (
                  <NotificationsPage token={auth.token} onUnreadChange={setUnreadNotifications} />
                ) : (
                  <div className="error-page">
                    <p>请先登录</p>
                    <Link to="/">返回首页</Link>
                  </div>
                )
              }
            />
            <Route
              path="/community"
              element={<CommunityPage token={auth?.token} userId={auth?.user.id} />}
            />
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
