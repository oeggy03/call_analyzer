import CoreMedia
import Foundation
import ScreenCaptureKit

struct CaptureOptions {
    let bundleID: String
    let spoolDirectory: URL
    let microphone: Bool
    let ocr: Bool
    let chunkSeconds: Int
}

private final class SampleBufferBox: @unchecked Sendable {
    let value: CMSampleBuffer

    init(_ value: CMSampleBuffer) {
        self.value = value
    }
}

final class CaptureEngine: NSObject, SCStreamOutput, SCStreamDelegate, @unchecked Sendable {
    private let options: CaptureOptions
    private let sink: EventSink
    private let sessionID = UUID().uuidString.lowercased()

    private let remoteQueue = BoundedSerialQueue(
        label: "call-analyzer.capture.remote-audio",
        capacity: 8
    )
    private let microphoneQueue = BoundedSerialQueue(
        label: "call-analyzer.capture.microphone-audio",
        capacity: 8
    )
    private let ocrQueue = BoundedSerialQueue(
        label: "call-analyzer.capture.ocr",
        capacity: 2
    )
    private let timestampNormalizer = TimestampNormalizer()

    private var stream: SCStream?
    private var remoteWriter: AudioChunkWriter?
    private var microphoneWriter: AudioChunkWriter?
    private var remoteConverter: AudioSampleConverter?
    private var microphoneConverter: AudioSampleConverter?
    private var ocrProcessor: OCRProcessor?

    private let stateLock = NSLock()
    private var started = false
    private var stopping = false
    private var stopRequested = false
    private var stopContinuation: CheckedContinuation<Void, Never>?
    private var reportedQueueDrops = Set<AudioSource>()
    private var reportedOCRError = false

    init(options: CaptureOptions, sink: EventSink) {
        self.options = options
        self.sink = sink
        super.init()
    }

    func start() async throws {
        guard !isStarted() else {
            throw CaptureFailure.stream("capture has already started")
        }

        let content = try await ShareableContentCatalog.current()
        let application = try ShareableContentCatalog.application(
            bundleID: options.bundleID,
            in: content
        )
        if options.microphone {
            try await ShareableContentCatalog.ensureMicrophonePermission()
        }
        let filter = try ShareableContentCatalog.filter(
            for: application,
            in: content
        )

        remoteWriter = try AudioChunkWriter(
            source: .remote,
            spoolDirectory: options.spoolDirectory,
            chunkSeconds: options.chunkSeconds
        )
        remoteConverter = try AudioSampleConverter()
        if options.microphone {
            microphoneWriter = try AudioChunkWriter(
                source: .microphone,
                spoolDirectory: options.spoolDirectory,
                chunkSeconds: options.chunkSeconds
            )
            microphoneConverter = try AudioSampleConverter()
        }
        if options.ocr {
            ocrProcessor = OCRProcessor()
        }

        let configuration = SCStreamConfiguration()
        configuration.capturesAudio = true
        configuration.sampleRate = 48_000
        configuration.channelCount = 2
        configuration.captureMicrophone = options.microphone
        configuration.queueDepth = 3
        configuration.showsCursor = false
        if options.ocr {
            configuration.width = 1_280
            configuration.height = 720
            configuration.pixelFormat = kCVPixelFormatType_32BGRA
            configuration.minimumFrameInterval = CMTime(value: 1, timescale: 1)
            configuration.scalesToFit = true
        }

        let stream = SCStream(
            filter: filter,
            configuration: configuration,
            delegate: self
        )
        self.stream = stream

        do {
            try stream.addStreamOutput(
                self,
                type: .audio,
                sampleHandlerQueue: DispatchQueue(
                    label: "call-analyzer.capture.stream-audio",
                    qos: .userInitiated
                )
            )
            if options.microphone {
                try stream.addStreamOutput(
                    self,
                    type: .microphone,
                    sampleHandlerQueue: DispatchQueue(
                        label: "call-analyzer.capture.stream-microphone",
                        qos: .userInitiated
                    )
                )
            }
            if options.ocr {
                try stream.addStreamOutput(
                    self,
                    type: .screen,
                    sampleHandlerQueue: DispatchQueue(
                        label: "call-analyzer.capture.stream-screen",
                        qos: .userInitiated
                    )
                )
            }
            try await stream.startCapture()
        } catch {
            throw CaptureFailure.stream(
                "ScreenCaptureKit could not start capture: \(error.localizedDescription)"
            )
        }

        markStarted()
        sink.emit(
            CaptureEvent(
                type: .ready,
                bundleID: options.bundleID,
                sessionID: sessionID
            )
        )
        sink.emit(
            CaptureEvent(
                type: .state,
                bundleID: options.bundleID,
                sessionID: sessionID,
                state: "running"
            )
        )
    }

    func waitForStopRequest() async {
        await withCheckedContinuation { continuation in
            stateLock.lock()
            if stopRequested {
                stateLock.unlock()
                continuation.resume()
                return
            }
            stopContinuation = continuation
            stateLock.unlock()
        }
    }

    func requestStop() {
        var continuation: CheckedContinuation<Void, Never>?
        stateLock.lock()
        if !stopRequested {
            stopRequested = true
            continuation = stopContinuation
            stopContinuation = nil
        }
        stateLock.unlock()
        continuation?.resume()
    }

    func stop() async {
        guard let stream = beginStopping() else {
            return
        }

        do {
            try await stream.stopCapture()
        } catch {
            sink.error(
                code: "stream_stop",
                message: "ScreenCaptureKit could not stop cleanly: \(error.localizedDescription)"
            )
        }

        remoteQueue.drain()
        microphoneQueue.drain()
        ocrQueue.drain()
        flush(writer: remoteWriter)
        flush(writer: microphoneWriter)
        sink.emit(
            CaptureEvent(
                type: .state,
                bundleID: options.bundleID,
                sessionID: sessionID,
                state: "stopped"
            )
        )
    }

    func stream(
        _ stream: SCStream,
        didOutputSampleBuffer sampleBuffer: CMSampleBuffer,
        of type: SCStreamOutputType
    ) {
        switch type {
        case .audio:
            enqueueAudio(sampleBuffer, source: .remote)
        case .microphone:
            enqueueAudio(sampleBuffer, source: .microphone)
        case .screen:
            enqueueScreen(sampleBuffer)
        @unknown default:
            sink.log("received an unknown ScreenCaptureKit output type")
        }
    }

    func stream(_ stream: SCStream, didStopWithError error: Error) {
        sink.error(
            code: "stream_stopped",
            message: "ScreenCaptureKit stopped the capture: \(error.localizedDescription)"
        )
        requestStop()
    }

    private func enqueueAudio(_ sampleBuffer: CMSampleBuffer, source: AudioSource) {
        let queue = source == .remote ? remoteQueue : microphoneQueue
        let boxedSampleBuffer = SampleBufferBox(sampleBuffer)
        guard queue.submit({ [weak self] in
            self?.processAudio(boxedSampleBuffer.value, source: source)
        }) else {
            reportQueueDrop(source)
            return
        }
    }

    private func enqueueScreen(_ sampleBuffer: CMSampleBuffer) {
        let boxedSampleBuffer = SampleBufferBox(sampleBuffer)
        guard ocrQueue.submit({ [weak self] in
            self?.processScreen(boxedSampleBuffer.value)
        }) else {
            sink.log("OCR queue is full; dropping a screen frame")
            return
        }
    }

    private func processAudio(_ sampleBuffer: CMSampleBuffer, source: AudioSource) {
        do {
            let converter: AudioSampleConverter?
            let writer: AudioChunkWriter?
            switch source {
            case .remote:
                converter = remoteConverter
                writer = remoteWriter
            case .microphone:
                converter = microphoneConverter
                writer = microphoneWriter
            }
            guard let converter, let writer else {
                return
            }
            let converted = try converter.convert(sampleBuffer)
            guard converted.frameCount > 0 else {
                return
            }
            let timestamp = timestampNormalizer.relativeMilliseconds(
                for: CMSampleBufferGetPresentationTimeStamp(sampleBuffer)
            )
            let chunks = try writer.append(
                converted.pcmData,
                frameCount: converted.frameCount,
                timestampMS: timestamp
            )
            emit(chunks)
        } catch {
            sink.error(
                code: "audio_processing",
                message: "\(source.rawValue) audio could not be converted: \(error.localizedDescription)"
            )
        }
    }

    private func processScreen(_ sampleBuffer: CMSampleBuffer) {
        guard
            let ocrProcessor,
            let pixelBuffer = CMSampleBufferGetImageBuffer(sampleBuffer)
        else {
            return
        }
        do {
            let timestamp = timestampNormalizer.relativeMilliseconds(
                for: CMSampleBufferGetPresentationTimeStamp(sampleBuffer)
            )
            if let result = try ocrProcessor.recognize(
                pixelBuffer: pixelBuffer,
                timestampMS: timestamp
            ) {
                sink.emit(
                    CaptureEvent(
                        type: .ocr,
                        bundleID: options.bundleID,
                        sessionID: sessionID,
                        text: result.text,
                        timestamp: result.timestampMS,
                        confidence: result.confidence
                    )
                )
            }
        } catch {
            stateLock.lock()
            let shouldReport = !reportedOCRError
            reportedOCRError = true
            stateLock.unlock()
            if shouldReport {
                sink.error(
                    code: "ocr_processing",
                    message: "Vision OCR failed: \(error.localizedDescription)"
                )
            }
        }
    }

    private func flush(writer: AudioChunkWriter?) {
        guard let writer else {
            return
        }
        do {
            emit(try writer.flush())
        } catch {
            sink.error(
                code: "audio_flush",
                message: "\(writer.source.rawValue) audio could not flush: \(error.localizedDescription)"
            )
        }
    }

    private func emit(_ chunks: [AudioChunkMetadata]) {
        for chunk in chunks {
            sink.emit(
                CaptureEvent(
                    type: .audioChunk,
                    bundleID: options.bundleID,
                    sessionID: sessionID,
                    source: chunk.source,
                    path: chunk.path.path,
                    startMS: chunk.startMS,
                    endMS: chunk.endMS,
                    sampleRate: chunk.sampleRate,
                    channels: chunk.channels
                )
            )
        }
    }

    private func reportQueueDrop(_ source: AudioSource) {
        stateLock.lock()
        let shouldReport = reportedQueueDrops.insert(source).inserted
        stateLock.unlock()
        if shouldReport {
            sink.error(
                code: "audio_queue_full",
                message: "\(source.rawValue) audio queue is full; samples are being dropped to bound memory use"
            )
        }
    }

    private func isStarted() -> Bool {
        stateLock.lock()
        defer { stateLock.unlock() }
        return started
    }

    private func markStarted() {
        stateLock.lock()
        started = true
        stateLock.unlock()
    }

    private func beginStopping() -> SCStream? {
        stateLock.lock()
        defer { stateLock.unlock() }
        guard !stopping else {
            return nil
        }
        stopping = true
        return stream
    }
}
