import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/forum.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { LiveProvider } from './lib/live'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Home from './pages/Home'
import ForumPage from './pages/ForumPage'
import TopicPage from './pages/TopicPage'
import NewTopic from './pages/NewTopic'
import Forums from './pages/Forums'
import ForumSettings from './pages/ForumSettings'
import Person from './pages/Person'
import Review from './pages/Review'
import { Bookmarks, Events, Inbox, Search } from './pages/Lists'
import { Join, PublicForum, PublicTopic } from './pages/Public'
import Settings from './pages/Settings'
import { DataProvider } from './lib/data'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <LiveProvider>{children}</LiveProvider>
}

/** Public pages work without an account; they wait for the sign-in check so a member gets "Open in Pocket Forum". */
function Open({ children }: { children: React.ReactNode }) {
  const { status } = useApp()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  return <>{children}</>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/p/:slug" element={<Open><ErrorBoundary where="public"><PublicForum /></ErrorBoundary></Open>} />
      <Route path="/p/:slug/c/:cat" element={<Open><ErrorBoundary where="public"><PublicForum /></ErrorBoundary></Open>} />
      <Route path="/p/:slug/t/:id" element={<Open><ErrorBoundary where="public"><PublicTopic /></ErrorBoundary></Open>} />
      <Route path="/join/:token" element={<Open><ErrorBoundary where="invite"><Join /></ErrorBoundary></Open>} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><DataProvider><Settings /></DataProvider></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="forum"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Home />} />
        <Route path="/forums" element={<Forums />} />
        <Route path="/f/:slug" element={<ForumPage />} />
        <Route path="/f/:slug/c/:cat" element={<ForumPage />} />
        <Route path="/f/:slug/settings/:tab?" element={<ForumSettings />} />
        <Route path="/t/:id/:n?" element={<TopicPage />} />
        <Route path="/new" element={<NewTopic />} />
        <Route path="/inbox" element={<Inbox />} />
        <Route path="/bookmarks" element={<Bookmarks />} />
        <Route path="/events" element={<Events />} />
        <Route path="/search" element={<Search />} />
        <Route path="/review" element={<Review />} />
        <Route path="/u/:id" element={<Person />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
