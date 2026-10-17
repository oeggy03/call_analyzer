@preconcurrency import AVFoundation
import CoreMedia
import Foundation

struct AudioChunkMetadata: Equatable {
    let source: AudioSource
    let path: URL
    let startMS: Int64
    let endMS: Int64
    let sampleRate: Int
    let channels: Int
}

final class WAVWriter {
    static let sampleRate = 16_000
    static let channels = 1
    static let bitsPerSample = 16
    static let bytesPerFrame = channels * bitsPerSample / 8

    private let handle: FileHandle
    private var dataByteCount: UInt64 = 0
    private var finalized = false

    init(url: URL) throws {
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        FileManager.default.createFile(atPath: url.path, contents: nil)
        handle = try FileHandle(forWritingTo: url)
        try handle.write(contentsOf: Data(repeating: 0, count: 44))
    }

    func append(_ data: Data) throws {
        guard !finalized else {
            throw AudioFileError.alreadyFinalized
        }
        guard data.count <= Int(UInt32.max) - Int(dataByteCount) else {
            throw AudioFileError.fileTooLarge
        }
        try handle.write(contentsOf: data)
        dataByteCount += UInt64(data.count)
    }

    func finalize() throws {
        guard !finalized else {
            return
        }
        finalized = true
        let header = Self.header(dataByteCount: dataByteCount)
        try handle.seek(toOffset: 0)
        try handle.write(contentsOf: header)
        try handle.close()
    }

    deinit {
        try? finalize()
    }

    static func header(dataByteCount: UInt64) -> Data {
        var header = Data()
        header.append(contentsOf: Array("RIFF".utf8))
        header.appendLE(UInt32(36 + min(dataByteCount, UInt64(UInt32.max))))
        header.append(contentsOf: Array("WAVE".utf8))
        header.append(contentsOf: Array("fmt ".utf8))
        header.appendLE(UInt32(16))
        header.appendLE(UInt16(1))
        header.appendLE(UInt16(channels))
        header.appendLE(UInt32(sampleRate))
        header.appendLE(UInt32(sampleRate * bytesPerFrame))
        header.appendLE(UInt16(bytesPerFrame))
        header.appendLE(UInt16(bitsPerSample))
        header.append(contentsOf: Array("data".utf8))
        header.appendLE(UInt32(min(dataByteCount, UInt64(UInt32.max))))
        return header
    }
}

enum AudioFileError: Error, LocalizedError {
    case alreadyFinalized
    case fileTooLarge
    case unsupportedFormat
    case conversionFailed(String)

    var errorDescription: String? {
        switch self {
        case .alreadyFinalized:
            return "audio chunk is already finalized"
        case .fileTooLarge:
            return "audio chunk exceeds the WAV 32-bit data-size limit"
        case .unsupportedFormat:
            return "audio sample buffer is not linear PCM"
        case .conversionFailed(let message):
            return "audio conversion failed: \(message)"
        }
    }
}

private extension Data {
    mutating func appendLE(_ value: UInt16) {
        append(UInt8(truncatingIfNeeded: value))
        append(UInt8(truncatingIfNeeded: value >> 8))
    }

    mutating func appendLE(_ value: UInt32) {
        append(UInt8(truncatingIfNeeded: value))
        append(UInt8(truncatingIfNeeded: value >> 8))
        append(UInt8(truncatingIfNeeded: value >> 16))
        append(UInt8(truncatingIfNeeded: value >> 24))
    }
}

final class AudioChunkWriter {
    let source: AudioSource
    let spoolDirectory: URL
    let chunkSeconds: Int

    private let chunkFrames: Int
    private var sequence = 0
    private var currentWriter: WAVWriter?
    private var currentPath: URL?
    private var currentStartMS: Int64 = 0
    private var currentFrames = 0

    init(source: AudioSource, spoolDirectory: URL, chunkSeconds: Int) throws {
        guard (1...3600).contains(chunkSeconds) else {
            throw AudioFileError.conversionFailed("chunk seconds must be between 1 and 3600")
        }
        self.source = source
        self.spoolDirectory = spoolDirectory
        self.chunkSeconds = chunkSeconds
        chunkFrames = chunkSeconds * WAVWriter.sampleRate
        try FileManager.default.createDirectory(
            at: spoolDirectory,
            withIntermediateDirectories: true
        )
    }

    @discardableResult
    func append(
        _ pcmData: Data,
        frameCount: Int,
        timestampMS: Int64
    ) throws -> [AudioChunkMetadata] {
        guard frameCount > 0 else {
            return []
        }
        let requiredBytes = frameCount * WAVWriter.bytesPerFrame
        guard pcmData.count >= requiredBytes else {
            throw AudioFileError.conversionFailed(
                "PCM buffer contains \(pcmData.count) bytes for \(frameCount) frames"
            )
        }

        var emitted: [AudioChunkMetadata] = []
        var sourceOffset = 0
        var framesRemaining = frameCount
        var nextTimestampMS = timestampMS

        while framesRemaining > 0 {
            if currentWriter == nil {
                currentStartMS = nextTimestampMS
                sequence += 1
                let filename = "\(source.rawValue)-\(sequence)-\(currentStartMS).wav"
                let path = spoolDirectory.appendingPathComponent(filename)
                currentPath = path
                currentWriter = try WAVWriter(url: path)
                currentFrames = 0
            }

            let framesToWrite = min(chunkFrames - currentFrames, framesRemaining)
            let byteCount = framesToWrite * WAVWriter.bytesPerFrame
            let range = sourceOffset..<(sourceOffset + byteCount)
            try currentWriter?.append(pcmData.subdata(in: range))
            currentFrames += framesToWrite
            sourceOffset += byteCount
            framesRemaining -= framesToWrite
            nextTimestampMS = currentStartMS + Int64(
                (currentFrames * 1000 + WAVWriter.sampleRate / 2) / WAVWriter.sampleRate
            )

            if currentFrames == chunkFrames {
                emitted.append(try finishCurrent())
            }
        }
        return emitted
    }

    func flush() throws -> [AudioChunkMetadata] {
        guard currentWriter != nil else {
            return []
        }
        return [try finishCurrent()]
    }

    private func finishCurrent() throws -> AudioChunkMetadata {
        guard let writer = currentWriter, let path = currentPath else {
            throw AudioFileError.conversionFailed("missing current chunk state")
        }
        try writer.finalize()
        let result = AudioChunkMetadata(
            source: source,
            path: path,
            startMS: currentStartMS,
            endMS: currentStartMS + Int64(
                (currentFrames * 1000 + WAVWriter.sampleRate / 2) / WAVWriter.sampleRate
            ),
            sampleRate: WAVWriter.sampleRate,
            channels: WAVWriter.channels
        )
        currentWriter = nil
        currentPath = nil
        currentFrames = 0
        return result
    }
}

struct ConvertedAudio {
    let pcmData: Data
    let frameCount: Int
}

private final class ConversionInputState: @unchecked Sendable {
    var supplied = false
}

final class AudioSampleConverter {
    private let outputFormat: AVAudioFormat

    init() throws {
        guard let format = AVAudioFormat(
            commonFormat: .pcmFormatInt16,
            sampleRate: Double(WAVWriter.sampleRate),
            channels: AVAudioChannelCount(WAVWriter.channels),
            interleaved: true
        ) else {
            throw AudioFileError.conversionFailed("cannot create 16 kHz mono output format")
        }
        outputFormat = format
    }

    func convert(_ sampleBuffer: CMSampleBuffer) throws -> ConvertedAudio {
        guard
            let formatDescription = CMSampleBufferGetFormatDescription(sampleBuffer),
            let streamDescription = CMAudioFormatDescriptionGetStreamBasicDescription(
                formatDescription
            ),
            streamDescription.pointee.mFormatID == kAudioFormatLinearPCM,
            let inputFormat = AVAudioFormat(streamDescription: streamDescription)
        else {
            throw AudioFileError.unsupportedFormat
        }

        let inputFrameCount = CMSampleBufferGetNumSamples(sampleBuffer)
        guard inputFrameCount > 0 else {
            return ConvertedAudio(pcmData: Data(), frameCount: 0)
        }
        guard let inputBuffer = AVAudioPCMBuffer(
            pcmFormat: inputFormat,
            frameCapacity: AVAudioFrameCount(inputFrameCount)
        ) else {
            throw AudioFileError.conversionFailed("cannot allocate input PCM buffer")
        }
        inputBuffer.frameLength = AVAudioFrameCount(inputFrameCount)

        do {
            try sampleBuffer.withAudioBufferList { source, _ in
                let destination = UnsafeMutableAudioBufferListPointer(
                    inputBuffer.mutableAudioBufferList
                )
                guard source.count == destination.count else {
                    throw AudioFileError.conversionFailed(
                        "audio buffer list count changed during conversion"
                    )
                }
                for index in 0..<source.count {
                    guard
                        let sourceData = source[index].mData,
                        let destinationData = destination[index].mData
                    else {
                        throw AudioFileError.conversionFailed(
                            "audio sample buffer has no backing data"
                        )
                    }
                    let byteCount = min(
                        Int(source[index].mDataByteSize),
                        Int(destination[index].mDataByteSize)
                    )
                    memcpy(destinationData, sourceData, byteCount)
                    destination[index].mDataByteSize = UInt32(byteCount)
                }
            }
        } catch let error as AudioFileError {
            throw error
        } catch {
            throw AudioFileError.conversionFailed(error.localizedDescription)
        }

        let estimatedFrameCount = max(
            1,
            Int(
                ceil(
                    Double(inputFrameCount) *
                        Double(WAVWriter.sampleRate) /
                        inputFormat.sampleRate
                )
            ) + 1024
        )
        guard let outputBuffer = AVAudioPCMBuffer(
            pcmFormat: outputFormat,
            frameCapacity: AVAudioFrameCount(estimatedFrameCount)
        ) else {
            throw AudioFileError.conversionFailed("cannot allocate output PCM buffer")
        }

        let inputState = ConversionInputState()
        var conversionError: NSError?
        let status = AVAudioConverter(
            from: inputFormat,
            to: outputFormat
        )?.convert(to: outputBuffer, error: &conversionError) { _, statusPointer in
            guard !inputState.supplied else {
                statusPointer.pointee = .endOfStream
                return nil
            }
            inputState.supplied = true
            statusPointer.pointee = .haveData
            return inputBuffer
        }

        guard let outputStatus = status else {
            throw AudioFileError.conversionFailed("cannot create audio converter")
        }
        guard outputStatus != .error, conversionError == nil else {
            throw AudioFileError.conversionFailed(
                conversionError?.localizedDescription ?? "converter returned an error"
            )
        }

        let frameCount = Int(outputBuffer.frameLength)
        guard frameCount > 0 else {
            return ConvertedAudio(pcmData: Data(), frameCount: 0)
        }
        guard
            let channelData = outputBuffer.int16ChannelData
        else {
            throw AudioFileError.conversionFailed("converter did not return int16 PCM")
        }
        let samples = channelData[0]
        return ConvertedAudio(
            pcmData: Data(
                bytes: samples,
                count: frameCount * WAVWriter.bytesPerFrame
            ),
            frameCount: frameCount
        )
    }
}

final class TimestampNormalizer: @unchecked Sendable {
    private let lock = NSLock()
    private var baseTimestamp: CMTime?

    func relativeMilliseconds(for timestamp: CMTime) -> Int64 {
        guard timestamp.isNumeric else {
            return 0
        }
        lock.lock()
        defer { lock.unlock() }
        if baseTimestamp == nil {
            baseTimestamp = timestamp
        }
        guard let baseTimestamp else {
            return 0
        }
        let seconds = CMTimeGetSeconds(CMTimeSubtract(timestamp, baseTimestamp))
        guard seconds.isFinite else {
            return 0
        }
        return max(0, Int64((seconds * 1000).rounded()))
    }
}
