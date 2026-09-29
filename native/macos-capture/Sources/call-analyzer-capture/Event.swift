import Foundation

enum CaptureEventType: String, Codable {
    case target
    case ready
    case audioChunk = "audio_chunk"
    case ocr
    case state
    case error
}

enum AudioSource: String, Codable, Hashable {
    case remote
    case microphone
}

struct CaptureWindow: Codable, Equatable {
    let windowID: UInt32
    let title: String
    let onScreen: Bool
    let active: Bool
    let x: Double
    let y: Double
    let width: Double
    let height: Double

    init(
        windowID: UInt32,
        title: String,
        onScreen: Bool,
        active: Bool,
        x: Double,
        y: Double,
        width: Double,
        height: Double
    ) {
        self.windowID = windowID
        self.title = title
        self.onScreen = onScreen
        self.active = active
        self.x = x
        self.y = y
        self.width = width
        self.height = height
    }

    private enum CodingKeys: String, CodingKey {
        case windowID = "window_id"
        case title
        case onScreen = "on_screen"
        case active
        case x
        case y
        case width
        case height
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(windowID, forKey: .windowID)
        try container.encode(title, forKey: .title)
        try container.encode(onScreen, forKey: .onScreen)
        try container.encode(active, forKey: .active)
        try container.encode(x, forKey: .x)
        try container.encode(y, forKey: .y)
        try container.encode(width, forKey: .width)
        try container.encode(height, forKey: .height)
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        windowID = try container.decode(UInt32.self, forKey: .windowID)
        title = try container.decode(String.self, forKey: .title)
        onScreen = try container.decode(Bool.self, forKey: .onScreen)
        active = try container.decode(Bool.self, forKey: .active)
        x = try container.decode(Double.self, forKey: .x)
        y = try container.decode(Double.self, forKey: .y)
        width = try container.decode(Double.self, forKey: .width)
        height = try container.decode(Double.self, forKey: .height)
    }
}

struct CaptureEvent: Codable, Equatable {
    let type: CaptureEventType
    var bundleID: String?
    var applicationName: String?
    var processID: Int32?
    var windows: [CaptureWindow]?
    var sessionID: String?
    var state: String?
    var source: AudioSource?
    var path: String?
    var startMS: Int64?
    var endMS: Int64?
    var sampleRate: Int?
    var channels: Int?
    var text: String?
    var timestamp: Int64?
    var confidence: Double?
    var code: String?
    var message: String?
    var actionable: Bool?

    init(
        type: CaptureEventType,
        bundleID: String? = nil,
        applicationName: String? = nil,
        processID: Int32? = nil,
        windows: [CaptureWindow]? = nil,
        sessionID: String? = nil,
        state: String? = nil,
        source: AudioSource? = nil,
        path: String? = nil,
        startMS: Int64? = nil,
        endMS: Int64? = nil,
        sampleRate: Int? = nil,
        channels: Int? = nil,
        text: String? = nil,
        timestamp: Int64? = nil,
        confidence: Double? = nil,
        code: String? = nil,
        message: String? = nil,
        actionable: Bool? = nil
    ) {
        self.type = type
        self.bundleID = bundleID
        self.applicationName = applicationName
        self.processID = processID
        self.windows = windows
        self.sessionID = sessionID
        self.state = state
        self.source = source
        self.path = path
        self.startMS = startMS
        self.endMS = endMS
        self.sampleRate = sampleRate
        self.channels = channels
        self.text = text
        self.timestamp = timestamp
        self.confidence = confidence
        self.code = code
        self.message = message
        self.actionable = actionable
    }

    private enum CodingKeys: String, CodingKey {
        case type
        case bundleID = "bundle_id"
        case applicationName = "application_name"
        case processID = "pid"
        case windows
        case sessionID = "session_id"
        case state
        case source
        case path
        case startMS = "start_ms"
        case endMS = "end_ms"
        case sampleRate = "sample_rate"
        case channels
        case text
        case timestamp
        case confidence
        case code
        case message
        case actionable
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(type, forKey: .type)
        try container.encodeIfPresent(bundleID, forKey: .bundleID)
        try container.encodeIfPresent(applicationName, forKey: .applicationName)
        try container.encodeIfPresent(processID, forKey: .processID)
        try container.encodeIfPresent(windows, forKey: .windows)
        try container.encodeIfPresent(sessionID, forKey: .sessionID)
        try container.encodeIfPresent(state, forKey: .state)
        try container.encodeIfPresent(source, forKey: .source)
        try container.encodeIfPresent(path, forKey: .path)
        try container.encodeIfPresent(startMS, forKey: .startMS)
        try container.encodeIfPresent(endMS, forKey: .endMS)
        try container.encodeIfPresent(sampleRate, forKey: .sampleRate)
        try container.encodeIfPresent(channels, forKey: .channels)
        try container.encodeIfPresent(text, forKey: .text)
        try container.encodeIfPresent(timestamp, forKey: .timestamp)
        try container.encodeIfPresent(confidence, forKey: .confidence)
        try container.encodeIfPresent(code, forKey: .code)
        try container.encodeIfPresent(message, forKey: .message)
        try container.encodeIfPresent(actionable, forKey: .actionable)
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        type = try container.decode(CaptureEventType.self, forKey: .type)
        bundleID = try container.decodeIfPresent(String.self, forKey: .bundleID)
        applicationName = try container.decodeIfPresent(String.self, forKey: .applicationName)
        processID = try container.decodeIfPresent(Int32.self, forKey: .processID)
        windows = try container.decodeIfPresent([CaptureWindow].self, forKey: .windows)
        sessionID = try container.decodeIfPresent(String.self, forKey: .sessionID)
        state = try container.decodeIfPresent(String.self, forKey: .state)
        source = try container.decodeIfPresent(AudioSource.self, forKey: .source)
        path = try container.decodeIfPresent(String.self, forKey: .path)
        startMS = try container.decodeIfPresent(Int64.self, forKey: .startMS)
        endMS = try container.decodeIfPresent(Int64.self, forKey: .endMS)
        sampleRate = try container.decodeIfPresent(Int.self, forKey: .sampleRate)
        channels = try container.decodeIfPresent(Int.self, forKey: .channels)
        text = try container.decodeIfPresent(String.self, forKey: .text)
        timestamp = try container.decodeIfPresent(Int64.self, forKey: .timestamp)
        confidence = try container.decodeIfPresent(Double.self, forKey: .confidence)
        code = try container.decodeIfPresent(String.self, forKey: .code)
        message = try container.decodeIfPresent(String.self, forKey: .message)
        actionable = try container.decodeIfPresent(Bool.self, forKey: .actionable)
    }
}

struct JSONLineEncoder {
    static func encode<T: Encodable>(_ value: T) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        return try encoder.encode(value)
    }
}

final class EventSink: @unchecked Sendable {
    private let output: FileHandle
    private let lock = NSLock()

    init(output: FileHandle = .standardOutput) {
        self.output = output
    }

    func emit(_ event: CaptureEvent) {
        do {
            var data = try JSONLineEncoder.encode(event)
            data.append(0x0A)
            lock.lock()
            defer { lock.unlock() }
            try output.write(contentsOf: data)
        } catch {
            log("failed to write JSON event: \(error.localizedDescription)")
        }
    }

    func log(_ message: String) {
        let data = Data((message + "\n").utf8)
        lock.lock()
        defer { lock.unlock() }
        try? FileHandle.standardError.write(contentsOf: data)
    }

    func error(
        code: String,
        message: String,
        actionable: Bool = false
    ) {
        emit(
            CaptureEvent(
                type: .error,
                code: code,
                message: message,
                actionable: actionable
            )
        )
        log("[\(code)] \(message)")
    }
}
