import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from './App'
import { BACKEND_EVENTS, createApi, createDemoApi, createUnavailableApi } from './lib/api'

function renderDemo() {
  return render(<App apiClient={createDemoApi()} />)
}

function mockDecodedPhoto(base64: string | string[]) {
  const values = Array.isArray(base64) ? [...base64] : [base64]
  class MockImage {
    naturalWidth = 1600
    naturalHeight = 900
    width = 1600
    height = 900
    onload: (() => void) | null = null
    onerror: (() => void) | null = null

    set src(_value: string) {
      queueMicrotask(() => this.onload?.())
    }
  }
  vi.stubGlobal('Image', MockImage)
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value: vi.fn(() => 'blob:photo'),
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value: vi.fn(),
  })
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
    drawImage: vi.fn(),
  } as unknown as CanvasRenderingContext2D)
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockImplementation(
    () => `data:image/jpeg;base64,${values.shift() ?? base64}`,
  )
}

async function openView(label: string) {
  fireEvent.click(await screen.findByRole('button', { name: new RegExp(`^${label}$`, 'i') }))
}

describe('Call Analyzer frontend', () => {
  afterEach(() => {
    window.go = undefined
    window.runtime = undefined
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
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

  it('pauses live transcript following when the user scrolls away', async () => {
    const api = createDemoApi()
    const snapshot = await api.getSnapshot()
    snapshot.transcript = Array.from({ length: 24 }, (_, index) => ({
      ...snapshot.transcript[index % snapshot.transcript.length],
      id: `scroll-line-${index}`,
      text: `滚动测试 ${index}`,
    }))
    vi.spyOn(api, 'getSnapshot').mockResolvedValue(snapshot)
    render(<App apiClient={api} />)

    const list = await screen.findByLabelText('Transcript lines')
    Object.defineProperty(list, 'scrollHeight', { configurable: true, value: 1200 })
    Object.defineProperty(list, 'clientHeight', { configurable: true, value: 300 })
    Object.defineProperty(list, 'scrollTop', { configurable: true, writable: true, value: 0 })
    fireEvent.scroll(list)

    expect(await screen.findByRole('button', { name: 'Resume live' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Resume live' }))
    expect(screen.queryByRole('button', { name: 'Resume live' })).not.toBeInTheDocument()
  })

  it('opens a past transcript from the workspace tab', async () => {
    renderDemo()
    fireEvent.click(await screen.findByRole('button', { name: 'Past transcripts' }))

    const lessons = within(await screen.findByRole('listbox', { name: 'Past transcripts' })).getAllByRole('option')
    fireEvent.click(lessons[0])
    expect(await screen.findByText('上次我们确认了发布日期。')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Past transcripts' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Live Lesson' }))
    expect(await screen.findByText('我们下周可以把这个方案再讨论一下。')).toBeInTheDocument()
    expect(screen.getByText('wǒ men xià zhōu kě yǐ bǎ zhè gè fāng àn zài tǎo lùn yī xià。')).toBeInTheDocument()
    expect(screen.queryByText('上次我们确认了发布日期。')).not.toBeInTheDocument()
  })

  it('gates lesson start on API-key readiness and links to Settings', async () => {
    const api = createDemoApi()
    await api.saveSettings({ openRouterKey: '' })
    render(<App apiClient={api} />)

    expect(await screen.findByRole('button', { name: /start lesson/i })).toBeDisabled()
    expect(screen.getByText(/Add a key in Settings/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Open Settings' }))
    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument()
  })

  it('displays actionable backend string errors without replacing them', async () => {
    const api = createDemoApi()
    vi.spyOn(api, 'requestCapturePermission').mockRejectedValue(
      'Screen Recording permission is required. Open System Settings and retry.',
    )
    render(<App apiClient={api} />)

    fireEvent.click(await screen.findByRole('button', { name: 'Check setup' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Screen Recording permission is required. Open System Settings and retry.',
    )
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

    expect(await screen.findByRole('alert')).toHaveTextContent(/no automatic cost stop/i)
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

  it('generates an editable Add Word draft, then saves it to Vocabulary', async () => {
    const api = createDemoApi()
    const generateManualVocabulary = vi.spyOn(api, 'generateManualVocabulary')
    render(<App apiClient={api} />)
    await openView('Add Word')

    fireEvent.change(screen.getByLabelText(/Chinese word \(simplified\)/i), { target: { value: '学习' } })
    fireEvent.click(screen.getByRole('button', { name: /Generate missing details/i }))

    expect(await screen.findByDisplayValue('xué xí')).toBeInTheDocument()
    expect(screen.getByDisplayValue('to study; to learn')).toBeInTheDocument()
    expect(screen.getByLabelText('Tags')).toHaveValue('')
    expect(screen.getByText(/qwen\/qwen3-32b/i)).toBeInTheDocument()
    expect(screen.getByText(/actual request cost: <\$0\.001/i)).toBeInTheDocument()
    expect(generateManualVocabulary).toHaveBeenCalledWith(expect.objectContaining({
      simplified: '学习',
      pinyin: '',
    }))

    fireEvent.change(screen.getByLabelText('English meaning'), { target: { value: 'to learn carefully' } })
    fireEvent.click(screen.getByRole('button', { name: /Save to Vocabulary/i }))

    expect(await screen.findByRole('heading', { name: 'Vocabulary' })).toBeInTheDocument()
    expect(await screen.findByText('学习')).toBeInTheDocument()
    expect(await screen.findByText('to learn carefully')).toBeInTheDocument()
    expect(await screen.findByText(/学习 saved to Vocabulary/i)).toBeInTheDocument()
  })

  it('keeps a single demo photo result in the suggestion list', async () => {
    mockDecodedPhoto('single')
    renderDemo()
    await openView('Add Word')

    const file = new File(['photo'], 'single.png', { type: 'image/png' })
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(fileInput, { target: { files: [file] } })
    await waitFor(() => expect(screen.getByAltText('Photo 1 preview')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Find words' }))

    expect(await screen.findByRole('checkbox', { name: '学习' })).toBeInTheDocument()
    expect(screen.getByLabelText(/Chinese word \(simplified\)/i)).toHaveValue('')
    expect(screen.getByText('Read with qwen/qwen3.7-flash')).toBeInTheDocument()
    expect(screen.getByText(/Actual request cost: \$0\.0001/i)).toBeInTheDocument()
  })

  it('reviews multiple demo photo entries and saves unchecked words only', async () => {
    mockDecodedPhoto('multiple')
    const api = createDemoApi()
    const initialSnapshot = await api.getSnapshot()
    initialSnapshot.vocabulary = initialSnapshot.vocabulary.filter((entry) => entry.simplified !== '重点')
    vi.spyOn(api, 'getSnapshot').mockResolvedValue(initialSnapshot)
    const saveManualVocabulary = vi.spyOn(api, 'saveManualVocabulary')
    render(<App apiClient={api} />)
    await openView('Add Word')

    const file = new File(['photo'], 'words.png', { type: 'image/png' })
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(fileInput, { target: { files: [file] } })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Find words' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'Find words' }))
    await waitFor(() => expect(screen.getByText('重点')).toBeInTheDocument())
    expect(screen.getByText('学习')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('checkbox', { name: '重点' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save selected' }))

    await waitFor(() => expect(saveManualVocabulary).toHaveBeenCalledTimes(1))
    expect(saveManualVocabulary).toHaveBeenCalledWith(expect.objectContaining({ simplified: '学习', aiGenerated: true }))
    expect(saveManualVocabulary).not.toHaveBeenCalledWith(expect.objectContaining({ simplified: '重点' }))
  })

  it('queues two photos, reads them sequentially, and merges duplicate words', async () => {
    mockDecodedPhoto(['photo-a', 'photo-b'])
    const api = createDemoApi()
    const recognizeVocabularyPhoto = vi.spyOn(api, 'recognizeVocabularyPhoto')
    render(<App apiClient={api} />)
    await openView('Add Word')

    const files = [
      new File(['photo-a'], 'photo-a.png', { type: 'image/png' }),
      new File(['photo-b'], 'photo-b.png', { type: 'image/png' }),
    ]
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(fileInput, { target: { files } })
    await waitFor(() => expect(screen.getByAltText('Photo 2 preview')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Find words' }))

    expect(await screen.findByText('学习')).toBeInTheDocument()
    expect(screen.getByText('方案')).toBeInTheDocument()
    expect(screen.getAllByText('重点')).toHaveLength(1)
    expect(recognizeVocabularyPhoto).toHaveBeenCalledTimes(2)
    expect(recognizeVocabularyPhoto).toHaveBeenNthCalledWith(1, 'photo-a')
    expect(recognizeVocabularyPhoto).toHaveBeenNthCalledWith(2, 'photo-b')
  })

  it('keeps successful suggestions when one photo fails', async () => {
    mockDecodedPhoto(['fail-photo', 'normal-photo'])
    renderDemo()
    await openView('Add Word')

    const files = [
      new File(['fail'], 'fail.png', { type: 'image/png' }),
      new File(['normal'], 'normal.png', { type: 'image/png' }),
    ]
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(fileInput, { target: { files } })
    await waitFor(() => expect(screen.getByAltText('Photo 2 preview')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Find words' }))

    expect(await screen.findByText('学习')).toBeInTheDocument()
    expect(await screen.findByText('This photo could not be read.')).toBeInTheDocument()
  })

  it('saves a complete manual form without an API key or generation', async () => {
    const api = createDemoApi()
    await api.saveSettings({ openRouterKey: '' })
    const generateManualVocabulary = vi.spyOn(api, 'generateManualVocabulary')
    render(<App apiClient={api} />)
    await openView('Add Word')

    fireEvent.change(screen.getByLabelText(/Chinese word \(simplified\)/i), { target: { value: '你好' } })
    fireEvent.change(screen.getByLabelText('Pinyin'), { target: { value: 'nǐ hǎo' } })
    fireEvent.change(screen.getByLabelText('English meaning'), { target: { value: 'hello' } })
    fireEvent.change(screen.getByLabelText('Chinese sample sentence'), { target: { value: '你好，老师！' } })
    fireEvent.change(screen.getByLabelText('Sample pinyin'), { target: { value: 'Nǐ hǎo, lǎo shī!' } })
    fireEvent.change(screen.getByLabelText('Sample English translation'), { target: { value: 'Hello, teacher!' } })

    expect(screen.getByRole('button', { name: /Save to Vocabulary/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: /Generate missing details/i })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /Save to Vocabulary/i }))

    expect(generateManualVocabulary).not.toHaveBeenCalled()
    expect(await screen.findByRole('heading', { name: 'Vocabulary' })).toBeInTheDocument()
    expect(await screen.findByText('你好')).toBeInTheDocument()
  })

  it('blocks incomplete generation without an API key and offers Settings', async () => {
    const api = createDemoApi()
    await api.saveSettings({ openRouterKey: '' })
    const generateManualVocabulary = vi.spyOn(api, 'generateManualVocabulary')
    render(<App apiClient={api} />)
    await openView('Add Word')

    fireEvent.change(screen.getByLabelText(/Chinese word \(simplified\)/i), { target: { value: '学习' } })
    const generateButton = screen.getByRole('button', { name: /Generate missing details/i })
    expect(generateButton).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Open Settings' })).toBeInTheDocument()
    expect(generateManualVocabulary).not.toHaveBeenCalled()
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
    const manualDraft = {
      simplified: '新词',
      traditional: '新詞',
      pinyin: 'xīn cí',
      meaning: 'new word',
      partOfSpeech: 'noun',
      classifier: '个',
      example: '这是新词。',
      examplePinyin: 'Zhè shì xīn cí.',
      exampleTranslation: 'This is a new word.',
      tags: ['work'],
      aiGenerated: true,
      model: 'qwen/qwen3-32b',
      cost: 0.0002,
    }
    const fakeApp = {
      GetAppSnapshot: vi.fn().mockResolvedValue(snapshot),
      GenerateManualVocabulary: vi.fn().mockResolvedValue(manualDraft),
      RecognizeVocabularyPhoto: vi.fn().mockResolvedValue({
        entries: [{
          simplified: '新词',
          traditional: '新詞',
          pinyin: 'xīn cí',
          meaning: 'new word',
          example: '这是新词。',
          examplePinyin: 'Zhè shì xīn cí.',
          exampleTranslation: 'This is a new word.',
          aiGenerated: true,
        }],
        model: 'qwen/qwen3.7-flash',
        cost: 0.0001,
      }),
      SaveManualVocabulary: vi.fn().mockResolvedValue(snapshot),
      ListLessonTranscripts: vi.fn().mockResolvedValue([{
        id: 'lesson-1',
        title: 'zoom',
        startedAt: '2026-09-29T14:00:00.000Z',
        segmentCount: 1,
        finalCost: 0.001,
      }]),
      GetLessonTranscript: vi.fn().mockResolvedValue({
        lesson: {
          id: 'lesson-1',
          title: 'zoom',
          startedAt: '2026-09-29T14:00:00.000Z',
          segmentCount: 1,
          finalCost: 0.001,
        },
        lines: snapshot.transcript.slice(0, 1),
      }),
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
    const manualInput = {
      simplified: '新词',
      traditional: '',
      pinyin: '',
      meaning: '',
      partOfSpeech: '',
      classifier: '',
      example: '',
      examplePinyin: '',
      exampleTranslation: '',
      tags: [],
      aiGenerated: false,
    }

    expect(runtimeApi.mode).toBe('runtime')
    await runtimeApi.getSnapshot()
    await runtimeApi.generateManualVocabulary(manualInput)
    await runtimeApi.recognizeVocabularyPhoto('photo-base64')
    await runtimeApi.saveManualVocabulary(manualInput)
    await runtimeApi.listLessonTranscripts(30)
    await runtimeApi.getLessonTranscript('lesson-1')
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

    expect(fakeApp.GenerateManualVocabulary).toHaveBeenCalledWith(manualInput)
    expect(fakeApp.RecognizeVocabularyPhoto).toHaveBeenCalledWith('photo-base64')
    expect(fakeApp.SaveManualVocabulary).toHaveBeenCalledWith(manualInput)
    expect(fakeApp.ListLessonTranscripts).toHaveBeenCalledWith(30)
    expect(fakeApp.GetLessonTranscript).toHaveBeenCalledWith('lesson-1')
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

  it('rejects an invalid runtime manual vocabulary draft', async () => {
    const snapshot = await createDemoApi().getSnapshot()
    window.go = {
      main: {
        App: {
          GetAppSnapshot: vi.fn().mockResolvedValue(snapshot),
          GenerateManualVocabulary: vi.fn().mockResolvedValue({
            simplified: '学习',
            model: 'qwen/qwen3-32b',
            cost: '0.0002',
          }),
        },
      },
    }

    const runtimeApi = createApi({ mode: 'runtime' })
    await expect(runtimeApi.generateManualVocabulary({ simplified: '学习' })).rejects.toThrow(
      'invalid manual vocabulary draft',
    )
  })
})
