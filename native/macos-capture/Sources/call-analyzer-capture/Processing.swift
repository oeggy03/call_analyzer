import CoreVideo
import Foundation

final class BoundedSerialQueue: @unchecked Sendable {
    private let queue: DispatchQueue
    private let slots: DispatchSemaphore

    init(label: String, capacity: Int, qos: DispatchQoS = .userInitiated) {
        precondition(capacity > 0)
        queue = DispatchQueue(label: label, qos: qos)
        slots = DispatchSemaphore(value: capacity)
    }

    @discardableResult
    func submit(_ work: @escaping @Sendable () -> Void) -> Bool {
        guard slots.wait(timeout: .now()) == .success else {
            return false
        }
        queue.async { [slots] in
            defer { slots.signal() }
            work()
        }
        return true
    }

    func drain() {
        queue.sync {}
    }
}

struct FrameHasher {
    private static let offsetBasis: UInt64 = 14_695_981_039_346_656_037
    private static let prime: UInt64 = 1_099_511_628_211

    static func hash(data: Data) -> UInt64 {
        var result = offsetBasis
        for byte in data {
            result ^= UInt64(byte)
            result &*= prime
        }
        return result
    }

    static func hash(pixelBuffer: CVPixelBuffer) -> UInt64 {
        CVPixelBufferLockBaseAddress(pixelBuffer, .readOnly)
        defer { CVPixelBufferUnlockBaseAddress(pixelBuffer, .readOnly) }

        var result = offsetBasis
        result = mix(result, UInt64(CVPixelBufferGetWidth(pixelBuffer)))
        result = mix(result, UInt64(CVPixelBufferGetHeight(pixelBuffer)))
        result = mix(result, UInt64(CVPixelBufferGetPixelFormatType(pixelBuffer)))

        guard let rawBuffer = CVPixelBufferGetBaseAddress(pixelBuffer) else {
            return result
        }
        let byteCount = CVPixelBufferGetDataSize(pixelBuffer)
        guard byteCount > 0 else {
            return result
        }
        let sampleCount = min(byteCount, 8_192)
        let sampleStride = max(1, byteCount / sampleCount)
        for offset in Swift.stride(from: 0, to: byteCount, by: sampleStride) {
            result = mix(result, UInt64(rawBuffer.load(fromByteOffset: offset, as: UInt8.self)))
        }
        return result
    }

    private static func mix(_ value: UInt64, _ byte: UInt64) -> UInt64 {
        var result = value
        result ^= byte
        result &*= prime
        return result
    }
}

struct FrameChangeGate {
    private(set) var lastHash: UInt64?

    init(lastHash: UInt64? = nil) {
        self.lastHash = lastHash
    }

    var hasPreviousHash: Bool {
        lastHash != nil
    }

    func isChanged(_ hash: UInt64) -> Bool {
        lastHash != hash
    }

    mutating func record(_ hash: UInt64) {
        lastHash = hash
    }

    mutating func shouldProcess(_ hash: UInt64) -> Bool {
        guard isChanged(hash) else {
            return false
        }
        record(hash)
        return true
    }
}

struct OCRChangeGate {
    private var frameGate = FrameChangeGate()
    private var lastProcessedTimestampMS: Int64?

    mutating func shouldProcess(hash: UInt64, timestampMS: Int64) -> Bool {
        guard frameGate.isChanged(hash) else {
            return false
        }
        if let lastProcessedTimestampMS,
           timestampMS >= lastProcessedTimestampMS,
           timestampMS - lastProcessedTimestampMS < 1_000 {
            return false
        }
        frameGate.record(hash)
        self.lastProcessedTimestampMS = timestampMS
        return true
    }
}
