import { useMemo, useState } from 'react'
import {
  ApplyWorkbookChanges,
  GetStatusSummary,
  InitializeWorkspace,
  OpenWorkbook,
  RefreshNow,
  SelectExistingWorkbookPath,
  SelectRootDirectory,
  SelectWorkbookSavePath,
  StartWatching,
  StopWatching,
} from '../wailsjs/go/wails/DesktopApp'
import { app } from '../wailsjs/go/models'
import './style.css'

type LogEntry = {
  time: string
  level: string
  source: string
  message: string
}

type ConflictResult = {
  recordId: string
  conflictType: string
  targetPath: string
  reason: string
  suggestion: string
}

type StatusSummary = {
  workspaceReady: boolean
  watcherActive: boolean
  snapshot: number
  fileCount: number
  conflicts: ConflictResult[]
  logs: LogEntry[]
}

const initialSummary: StatusSummary = {
  workspaceReady: false,
  watcherActive: false,
  snapshot: 0,
  fileCount: 0,
  conflicts: [],
  logs: [],
}

type WorkbookMode = 'create' | 'existing' | ''

function App() {
  const [rootDir, setRootDir] = useState('')
  const [workbookPath, setWorkbookPath] = useState('')
  const [summary, setSummary] = useState<StatusSummary>(initialSummary)
  const [busy, setBusy] = useState(false)
  const [errorMessage, setErrorMessage] = useState('')
  const [successMessage, setSuccessMessage] = useState('')
  const [workbookMode, setWorkbookMode] = useState<WorkbookMode>('')

  const activityItems = useMemo(
    () => [
      {
        title: '工作区状态',
        text: summary.workspaceReady
          ? `工作区已初始化，当前纳管 ${summary.fileCount} 个文件。`
          : '尚未初始化工作区，请先选择根目录，再新建工作簿并导出或选择现有工作簿。',
      },
      {
        title: '监听状态',
        text: summary.watcherActive ? '监听器已启动，保存 Excel 后会自动同步。' : '监听器当前未启动，可在监听状态卡片右上角手动开启。',
      },
      {
        title: '待人工处理',
        text:
          summary.conflicts.length > 0
            ? `当前存在 ${summary.conflicts.length} 个冲突项，已写入工作簿 D 列。`
            : '当前没有待处理冲突。',
      },
    ],
    [summary],
  )

  const applySummary = (nextSummary: app.StatusSummary | StatusSummary) => {
    setSummary({
      workspaceReady: nextSummary.workspaceReady,
      watcherActive: nextSummary.watcherActive,
      snapshot: nextSummary.snapshot,
      fileCount: nextSummary.fileCount,
      conflicts: nextSummary.conflicts ?? [],
      logs: (nextSummary.logs ?? [])
        .map((entry) => ({
          time: String(entry.time ?? ''),
          level: entry.level,
          source: entry.source,
          message: entry.message,
        }))
        .reverse(),
    })
  }

  const refreshSummary = async () => {
    const latest = await GetStatusSummary()
    applySummary(latest)
    return latest
  }

  const runAction = async <T,>(action: () => Promise<T>, successTip?: string, refreshAfter = true) => {
    setBusy(true)
    setErrorMessage('')
    if (successTip) {
      setSuccessMessage('')
    }
    try {
      const result = await action()
      if (refreshAfter) {
        await refreshSummary()
      }
      if (successTip) {
        setSuccessMessage(successTip)
      }
      return result
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      setErrorMessage(message)
      throw error
    } finally {
      setBusy(false)
    }
  }

  const ensureRootDir = () => {
    if (!rootDir.trim()) {
      setErrorMessage('请先选择根目录。')
      return false
    }
    return true
  }

  const pickRootDirectory = async () => {
    const selected = await runAction(() => SelectRootDirectory(rootDir), '已选择根目录', false)
    if (selected) {
      setRootDir(selected)
    }
  }

  const createWorkbookAndExport = async () => {
    if (!ensureRootDir()) {
      return
    }
    const selected = await runAction(() => SelectWorkbookSavePath(workbookPath), undefined, false)
    if (!selected) {
      return
    }
    setWorkbookPath(selected)
    setWorkbookMode('create')
    await runAction(async () => {
      const nextSummary = await InitializeWorkspace(rootDir.trim(), selected.trim())
      if (!nextSummary.watcherActive) {
        await StartWatching()
      }
      await OpenWorkbook(selected.trim())
      return nextSummary
    }, '已新建工作簿')
  }

  const selectExistingWorkbook = async () => {
    if (!ensureRootDir()) {
      return
    }
    const selected = await runAction(() => SelectExistingWorkbookPath(workbookPath), undefined, false)
    if (!selected) {
      return
    }
    setWorkbookPath(selected)
    setWorkbookMode('existing')
    await runAction(async () => {
      const nextSummary = await InitializeWorkspace(rootDir.trim(), selected.trim())
      if (!nextSummary.watcherActive) {
        await StartWatching()
      }
      await OpenWorkbook(selected.trim())
      return nextSummary
    }, '已选择已有工作簿')
  }

  const refreshWorkspace = async () => {
    await runAction(() => RefreshNow(), '已同步文件状态')
  }

  const applyWorkbookChanges = async () => {
    await runAction(() => ApplyWorkbookChanges(), '已将 Excel 中的修改应用到实际文件')
  }

  const toggleWatching = async () => {
    if (!summary.workspaceReady) {
      setErrorMessage('请先完成工作区初始化。')
      return
    }
    if (summary.watcherActive) {
      await runAction(() => StopWatching(), '监听器已停止')
      return
    }
    await runAction(() => StartWatching(), '监听器已启动')
  }

  const openWorkbookLocation = async () => {
    if (!workbookPath.trim()) {
      setErrorMessage('请先选择工作簿路径。')
      return
    }
    await runAction(() => OpenWorkbook(workbookPath.trim()), '已调用系统默认程序打开工作簿', false)
  }

  return (
    <div className="app-shell">
      <header className="topbar glass-card">
        <div className="topbar-brand">
          <div className="brand-title-row">
            <div className="brand-copy">
              <h1 className="brand-name">ExceLabel</h1>
              <p className="brand-subtitle">Excel 与文件系统双向同步控制台</p>
            </div>
            <div className="topbar-notes">
              <div className="note-item">• 统计文件夹下文件名到 Excel</div>
              <div className="note-item">• 在 Excel 中进行批量文件改名</div>
            </div>
          </div>
        </div>
        <div className="topbar-side">
          <div className="topbar-actions">
            <div className="action-stack">
              <button className="success-btn" onClick={applyWorkbookChanges} disabled={busy || !summary.workspaceReady}>
                应用 Excel 修改
              </button>
              <span className="action-hint">Excel-&gt;文件</span>
            </div>
            <div className="action-stack">
              <button className="primary-btn" onClick={refreshWorkspace} disabled={busy || !summary.workspaceReady}>
                同步文件状态
              </button>
              <span className="action-hint">文件-&gt;Excel</span>
            </div>
          </div>
        </div>
      </header>

      <main className="main-grid">
        <aside className="left-panel glass-card">
          <section>
            <h2>工作区配置</h2>
            <label>
              根目录
              <div className="input-with-action">
                <input value={rootDir} onChange={(event) => setRootDir(event.target.value)} placeholder="请选择需要扫描的目录" />
                <button className="ghost-btn inline-btn" onClick={pickRootDirectory} disabled={busy}>
                  选择目录
                </button>
              </div>
            </label>
            <label>
              工作簿路径
              <input
                value={workbookPath}
                onChange={(event) => setWorkbookPath(event.target.value)}
                placeholder="新建或选择现有 Excel 工作簿后会显示在这里"
              />
            </label>
            <div className="button-column workspace-actions">
              <div className="button-row workbook-mode-actions">
                <button
                  className={`workbook-select-btn ${workbookMode === 'create' ? 'active' : ''}`}
                  onClick={createWorkbookAndExport}
                  disabled={busy}
                >
                  新建工作簿
                </button>
                <button
                  className={`workbook-select-btn ${workbookMode === 'existing' ? 'active' : ''}`}
                  onClick={selectExistingWorkbook}
                  disabled={busy}
                >
                  已有工作簿
                </button>
              </div>
              <button className="ghost-btn workbook-open-btn" onClick={openWorkbookLocation} disabled={busy || !workbookPath.trim()}>
                打开工作簿
              </button>
            </div>
          </section>

          <section>
            <h2>同步策略</h2>
            <div className="setting-item setting-item-compact">
              <span>工作区状态</span>
              <div className="setting-value setting-status">{summary.workspaceReady ? '已初始化' : '未初始化'}</div>
            </div>
            <div className="setting-item setting-item-multiline">
              <span>工作簿路径</span>
              <div className="setting-value-wrap">
                <div className="setting-value-text">{workbookPath || '未选择'}</div>
              </div>
            </div>
            <div className="setting-item setting-item-multiline">
              <span>根目录</span>
              <div className="setting-value-wrap">
                <div className="setting-value-text">{rootDir || '未选择'}</div>
              </div>
            </div>
          </section>

        </aside>

        <section className="center-panel">
          {(errorMessage || successMessage) && (
            <section className={`message-panel glass-card ${errorMessage ? 'error-panel' : 'success-panel'}`}>
              <strong>{errorMessage ? '操作失败' : '操作完成'}</strong>
              <span>{errorMessage || successMessage}</span>
            </section>
          )}

          <div className="stats-grid">
            <article className="stat-card glass-card">
              <span>文件总数</span>
              <strong className={`stat-value ${String(summary.fileCount.toLocaleString('zh-CN')).length > 10 ? 'compact' : ''}`}>
                {summary.fileCount.toLocaleString('zh-CN')}
              </strong>
            </article>
            <article className="stat-card glass-card">
              <span>当前快照</span>
              <strong className={`stat-value ${String(summary.snapshot > 0 ? summary.snapshot.toLocaleString('zh-CN') : '未初始化').length > 10 ? 'compact' : ''}`}>
                {summary.snapshot > 0 ? summary.snapshot.toLocaleString('zh-CN') : '未初始化'}
              </strong>
            </article>
            <article className="stat-card glass-card">
              <span>冲突数量</span>
              <strong className={`stat-value ${String(summary.conflicts.length.toLocaleString('zh-CN')).length > 10 ? 'compact' : ''}`}>
                {summary.conflicts.length.toLocaleString('zh-CN')}
              </strong>
            </article>
            <article className="stat-card glass-card listener-card">
              <div className="listener-card-header">
                <span>监听状态</span>
                <button
                  className={`listener-toggle ${summary.watcherActive ? 'stop' : 'start'}`}
                  onClick={toggleWatching}
                  disabled={busy || !summary.workspaceReady}
                >
                  {summary.watcherActive ? '停止' : '开始'}
                </button>
              </div>
              <strong className="stat-value">{summary.watcherActive ? '运行中' : '已停止'}</strong>
            </article>
          </div>

          <section className="activity-panel glass-card">
            <div className="section-header">
              <h2>最近同步概览</h2>
            </div>
            <div className="activity-list">
              {activityItems.map((item) => (
                <article key={item.title}>
                  <strong>{item.title}</strong>
                  <p>{item.text}</p>
                </article>
              ))}
            </div>
          </section>

          <section className="log-panel glass-card">
            <div className="section-header">
              <h2>日志控制台</h2>
              <span>{summary.logs.length} 条记录</span>
            </div>
            <div className="log-list scroll-region">
              {summary.logs.length === 0 ? (
                <div className="empty-state">尚无日志，请先执行初始化或同步操作。</div>
              ) : (
                summary.logs.map((log, index) => (
                  <div className="log-row" key={`${log.time}-${index}`}>
                    <span>{log.time ? new Date(log.time).toLocaleTimeString() : '--:--:--'}</span>
                    <span className={`level ${log.level}`}>{log.level}</span>
                    <span>{log.source}</span>
                    <span>{log.message}</span>
                  </div>
                ))
              )}
            </div>
          </section>
        </section>

        <div className="right-column">
          <section className="usage-tip-panel glass-card">
            <h2>使用提示</h2>
            <div className="setting-item setting-item-multiline usage-tip-list">
              <div className="usage-tip-item">• 在 Excel 中改名并保存后点击“应用 Excel 修改”。</div>
              <div className="usage-tip-item">• 磁盘变化后点击“同步文件状态”。</div>
              <div className="usage-tip-item">• 应用同步文件状态前需要关闭对应工作簿。</div>
            </div>
          </section>

          <aside className="right-panel glass-card">
            <div className="section-header">
              <h2>冲突与任务详情</h2>
              <span>{summary.conflicts.length} 项待处理</span>
            </div>
            <div className="conflict-list scroll-region">
              {summary.conflicts.length === 0 ? (
                <div className="empty-state">当前没有冲突项。</div>
              ) : (
                summary.conflicts.map((conflict) => (
                  <article key={conflict.recordId} className="conflict-card">
                    <div className="conflict-title-row">
                      <strong>{conflict.conflictType}</strong>
                      <span>{conflict.recordId}</span>
                    </div>
                    <p>{conflict.reason}</p>
                    <small>{conflict.targetPath}</small>
                    <div className="conflict-footer">建议：{conflict.suggestion}</div>
                  </article>
                ))
              )}
            </div>
          </aside>
        </div>
      </main>
    </div>
  )
}

export default App
