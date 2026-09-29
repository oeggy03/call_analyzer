import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App } from './App'
import { BACKEND_EVENTS, createApi, createDemoApi, createUnavailableApi } from './lib/api'

function renderDemo() {
  return render(<App apiClient={createDemoApi()} />)
}

async function openView(label: string) {
  fireEvent.click(await screen.findByRole('button', { name: new RegExp(`^${label}$`, 'i') }))
}

describe('Call Analyzer frontend', () => {
  afterEach(() => {
    window.go = undefined
  })

  it('gates lesson start on consent and allows marking a live moment', async () => {
    renderDemo()

    const startButton = await screen.findByRole('button', { name: /start lesson/i })
    expect(startButton).toBeDisabled()

    fireEvent.click(screen.getByRole('checkbox', { name: /consent/i }))
    expect(startButton).toBeEnabled()
    fireEvent.click(startButton)

    expect(await screen.findByText('Lesson in progress')).toBeInTheDocument()
    const markButton = screen.getByRole('button', { name: /mark moment/i })
    expect(markButton).toBeEnabled()
    fireEvent.click(markButton)

    expect(await screen.findByText('Moment marked. It is waiting in Candidate Inbox.')).toBeInTheDocument()
  })

  it('confirms and rejects candidates with safe feedback', async () => {
    renderDemo()
    await openView('Candidate Inbox')

    const confirmButton = (await screen.findAllByRole('button', { name: 'Confirm' }))[0]
    fireEvent.click(confirmButton)
    expect(await screen.findByText(/added to Vocabulary/)).toBeInTheDocument()

    const rejectButton = screen.getAllByRole('button', { name: 'Reject' })[0]
    fireEvent.click(rejectButton)
    expect(await screen.findByText(/rejected\./)).toBeInTheDocument()
  })

  it('moves keyboard focus into the candidate edit dialog', async () => {
    renderDemo()
    await openView('Candidate Inbox')

    fireEvent.click((await screen.findAllByRole('button', { name: 'Edit' }))[0])

    expect(await screen.findByRole('dialog', { name: /重点/i })).toBeInTheDocument()
    expect(screen.getByLabelText('Pinyin')).toHaveFocus()
  })

  it('filters vocabulary by search text', async () => {
    renderDemo()
    await openView('Vocabulary')

    expect(await screen.findByText('重点')).toBeInTheDocument()
    const search = screen.getByRole('searchbox', { name: /search vocabulary/i })
    fireEvent.change(search, { target: { value: '方案' } })

    expect(screen.getByText('方案')).toBeInTheDocument()
    expect(screen.queryByText('重点')).not.toBeInTheDocument()
  })

  it('shows a useful unavailable-backend state', async () => {
    render(<App apiClient={createUnavailableApi()} />)

    expect(await screen.findByText('Backend unavailable')).toBeInTheDocument()
    expect(screen.getByText(/Start the Wails desktop runtime/i)).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: /start lesson/i })).toBeDisabled())
  })

  it('passes the exact positional Wails argument shapes', async () => {
    const snapshot = await createDemoApi().getSnapshot()
    const fakeApp = {
      GetAppSnapshot: vi.fn().mockResolvedValue(snapshot),
      RequestCapturePermission: vi.fn().mockResolvedValue(snapshot),
      StartLesson: vi.fn().mockResolvedValue(snapshot),
      StopLesson: vi.fn().mockResolvedValue(snapshot),
      MarkMoment: vi.fn().mockResolvedValue(snapshot),
      ConfirmCandidate: vi.fn().mockResolvedValue(snapshot),
      EditCandidate: vi.fn().mockResolvedValue(snapshot),
      RejectCandidate: vi.fn().mockResolvedValue(snapshot),
      MergeCandidates: vi.fn().mockResolvedValue(snapshot),
      SaveSettings: vi.fn().mockResolvedValue(snapshot),
      Refresh: vi.fn().mockResolvedValue(snapshot),
    }
    window.go = { main: { App: fakeApp } }

    const runtimeApi = createApi({ mode: 'runtime' })
    const editPatch = { pinyin: 'xīn pīn yīn', meaning: 'new meaning' }
    const settingsPatch = { sttModel: 'qwen/qwen3-asr-1.7b' }

    expect(runtimeApi.mode).toBe('runtime')
    await runtimeApi.getSnapshot()
    await runtimeApi.requestCapturePermission()
    await runtimeApi.startLesson({ target: 'zoom', consent: true })
    await runtimeApi.stopLesson()
    await runtimeApi.markMoment()
    await runtimeApi.confirmCandidate('candidate-1')
    await runtimeApi.editCandidate('candidate-1', editPatch)
    await runtimeApi.rejectCandidate('candidate-1')
    await runtimeApi.mergeCandidates('candidate-1', 'candidate-2')
    await runtimeApi.saveSettings(settingsPatch)
    await runtimeApi.refresh()

    expect(fakeApp.StartLesson).toHaveBeenCalledWith({ target: 'zoom', consent: true })
    expect(fakeApp.ConfirmCandidate).toHaveBeenCalledWith('candidate-1')
    expect(fakeApp.EditCandidate).toHaveBeenCalledWith('candidate-1', editPatch)
    expect(fakeApp.RejectCandidate).toHaveBeenCalledWith('candidate-1')
    expect(fakeApp.MergeCandidates).toHaveBeenCalledWith('candidate-1', 'candidate-2')
    expect(fakeApp.SaveSettings).toHaveBeenCalledWith(settingsPatch)
    expect(BACKEND_EVENTS.snapshotChanged).toBe('call_analyzer:snapshot_changed')
  })

  it('rejects an invalid runtime snapshot instead of inventing state', async () => {
    window.go = {
      main: {
        App: {
          GetAppSnapshot: vi.fn().mockResolvedValue({}),
        },
      },
    }

    await expect(createApi({ mode: 'runtime' }).getSnapshot()).rejects.toThrow('invalid snapshot')
  })
})
