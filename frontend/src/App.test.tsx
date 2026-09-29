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
    window.runtime = undefined
    vi.restoreAllMocks()
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

  it('gates lesson start on API-key readiness and links to Settings', async () => {
    const api = createDemoApi()
    await api.saveSettings({ openRouterKey: '' })
    render(<App apiClient={api} />)

    expect(await screen.findByRole('button', { name: /start lesson/i })).toBeDisabled()
    expect(screen.getByText(/API key required/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Open Settings' }))
    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument()
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

  it('closes candidate dialogs with Escape', async () => {
    renderDemo()
    await openView('Candidate Inbox')
    fireEvent.click((await screen.findAllByRole('button', { name: 'Edit' }))[0])
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('submits the complete candidate edit patch and validates required fields', async () => {
    const api = createDemoApi()
    const editCandidate = vi.spyOn(api, 'editCandidate')
    render(<App apiClient={api} />)
    await openView('Candidate Inbox')
    fireEvent.click((await screen.findAllByRole('button', { name: 'Edit' }))[0])

    fireEvent.change(screen.getByLabelText('Hanzi'), { target: { value: '重点新' } })
    fireEvent.change(screen.getByLabelText('Traditional'), { target: { value: '重點新' } })
    fireEvent.change(screen.getByLabelText('Pinyin'), { target: { value: 'zhòngdiǎn xīn' } })
    fireEvent.change(screen.getByLabelText('Meaning'), { target: { value: 'new meaning' } })
    fireEvent.change(screen.getByLabelText('Part of speech'), { target: { value: 'noun' } })
    fireEvent.change(screen.getByLabelText('Classifier'), { target: { value: '个' } })
    fireEvent.change(screen.getByLabelText('Sample sentence'), { target: { value: '这是一个新例句。' } })
    fireEvent.change(screen.getByLabelText('Sample pinyin'), { target: { value: 'Zhè shì yí ge xīn lìjù.' } })
    fireEvent.change(screen.getByLabelText('Translation'), { target: { value: 'This is a new example.' } })
    fireEvent.change(screen.getByLabelText(/Tags/), { target: { value: 'work, edited' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(editCandidate).toHaveBeenCalledWith(expect.any(String), {
      simplified: '重点新',
      traditional: '重點新',
      pinyin: 'zhòngdiǎn xīn',
      meaning: 'new meaning',
      partOfSpeech: 'noun',
      classifier: '个',
      example: '这是一个新例句。',
      examplePinyin: 'Zhè shì yí ge xīn lìjù.',
      exampleTranslation: 'This is a new example.',
      tags: ['work', 'edited'],
    }))

    fireEvent.click((await screen.findAllByRole('button', { name: 'Edit' }))[0])
    fireEvent.change(screen.getByLabelText('Meaning'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/required fields: meaning/i)
    expect(editCandidate).toHaveBeenCalledTimes(1)
  })

  it('clears the API key with confirmation and status feedback', async () => {
    const api = createDemoApi()
    const saveSettings = vi.spyOn(api, 'saveSettings')
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<App apiClient={api} />)
    await openView('Settings')

    fireEvent.click(screen.getByRole('button', { name: 'Clear API key' }))
    await waitFor(() => expect(saveSettings).toHaveBeenCalledWith({ openRouterKey: '' }))
    expect(await screen.findByText(/API key cleared/i)).toBeInTheDocument()
  })

  it('deletes the last lesson only after confirmation', async () => {
    const api = createDemoApi()
    const deleteLastLesson = vi.spyOn(api, 'deleteLastLesson')
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<App apiClient={api} />)
    await openView('Settings')

    const deleteButton = screen.getByRole('button', { name: /delete last lesson data/i })
    expect(deleteButton).toBeEnabled()
    fireEvent.click(deleteButton)

    await waitFor(() => expect(deleteLastLesson).toHaveBeenCalledWith())
    expect(await screen.findByText(/Last lesson data deleted/i)).toBeInTheDocument()
  })

  it('shows cost warning states prominently in Live Lesson', async () => {
    const api = createDemoApi()
    const snapshot = await api.getSnapshot()
    snapshot.cost.warning = true
    vi.spyOn(api, 'getSnapshot').mockResolvedValue(snapshot)
    render(<App apiClient={api} />)

    expect(await screen.findByRole('alert')).toHaveTextContent(/Approaching the hard cost limit/i)
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
    expect(screen.getByText(/Start the local desktop runtime/i)).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: /start lesson/i })).toBeDisabled())
  })

  it('clearly labels the in-memory demo adapter', async () => {
    renderDemo()
    expect(await screen.findByText('Demo mode is active')).toBeInTheDocument()
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
      DeleteLastLesson: vi.fn().mockResolvedValue(snapshot),
      Refresh: vi.fn().mockResolvedValue(snapshot),
    }
    window.go = { main: { App: fakeApp } }

    const runtimeApi = createApi({ mode: 'runtime' })
    const editPatch = {
      simplified: '新词',
      traditional: '新詞',
      pinyin: 'xīn cí',
      meaning: 'new meaning',
      partOfSpeech: 'noun',
      classifier: '个',
      example: '这是新例句。',
      examplePinyin: 'Zhè shì xīn lìjù.',
      exampleTranslation: 'This is a new example.',
      tags: ['work'],
    }
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
    await runtimeApi.deleteLastLesson()
    await runtimeApi.refresh()

    expect(fakeApp.StartLesson).toHaveBeenCalledWith({ target: 'zoom', consent: true })
    expect(fakeApp.ConfirmCandidate).toHaveBeenCalledWith('candidate-1')
    expect(fakeApp.EditCandidate).toHaveBeenCalledWith('candidate-1', editPatch)
    expect(fakeApp.RejectCandidate).toHaveBeenCalledWith('candidate-1')
    expect(fakeApp.MergeCandidates).toHaveBeenCalledWith('candidate-1', 'candidate-2')
    expect(fakeApp.SaveSettings).toHaveBeenCalledWith(settingsPatch)
    expect(fakeApp.DeleteLastLesson).toHaveBeenCalledWith()
    expect(BACKEND_EVENTS.snapshotChanged).toBe('call_analyzer:snapshot_changed')
  })

  it('applies lightweight level events and rejects stale snapshots', async () => {
    const snapshot = await createDemoApi().getSnapshot()
    const stale = { ...snapshot, lastUpdated: '2020-01-01T00:00:00.000Z' }
    const callbacks = new Map<string, (...args: unknown[]) => void>()
    window.runtime = {
      EventsOn: (name, callback) => {
        callbacks.set(name, callback)
        return () => callbacks.delete(name)
      },
    }
    window.go = {
      main: {
        App: {
          GetAppSnapshot: vi.fn().mockResolvedValue(snapshot),
        },
      },
    }
    const runtimeApi = createApi({ mode: 'runtime' })
    await runtimeApi.getSnapshot()
    const listener = vi.fn()
    const unsubscribe = runtimeApi.subscribe(listener)

    callbacks.get(BACKEND_EVENTS.captureLevels)?.({ mic: 0.25, remote: 0.75 })
    expect(listener).toHaveBeenLastCalledWith(expect.objectContaining({
      levels: { mic: 0.25, remote: 0.75 },
    }))
    callbacks.get(BACKEND_EVENTS.snapshotChanged)?.(stale)
    expect(listener).toHaveBeenCalledTimes(1)
    unsubscribe()
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
