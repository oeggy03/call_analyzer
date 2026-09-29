import Foundation
import XCTest
@testable import call_analyzer_capture

final class CaptureTests: XCTestCase {
    func testWAVWriterProducesPCM16MonoHeader() throws {
        let directory = try temporaryDirectory()
        let url = directory.appendingPathComponent("sample.wav")
        let writer = try WAVWriter(url: url)
        let samples = Data([0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80])
        try writer.append(samples)
        try writer.finalize()

        let bytes = try Data(contentsOf: url)
        XCTAssertEqual(String(decoding: bytes.prefix(4), as: UTF8.self), "RIFF")
        XCTAssertEqual(String(decoding: bytes[8..<12], as: UTF8.self), "WAVE")
        XCTAssertEqual(String(decoding: bytes[12..<16], as: UTF8.self), "fmt ")
        XCTAssertEqual(readUInt16(bytes, at: 20), 1)
        XCTAssertEqual(readUInt16(bytes, at: 22), 1)
        XCTAssertEqual(readUInt32(bytes, at: 24), 16_000)
        XCTAssertEqual(readUInt32(bytes, at: 28), 32_000)
        XCTAssertEqual(readUInt16(bytes, at: 32), 2)
        XCTAssertEqual(readUInt16(bytes, at: 34), 16)
        XCTAssertEqual(String(decoding: bytes[36..<40], as: UTF8.self), "data")
        XCTAssertEqual(readUInt32(bytes, at: 40), 6)
        XCTAssertEqual(Array(bytes[44...]), Array(samples))
    }

    func testJSONEncodingUsesMachineReadableSnakeCaseFields() throws {
        let event = CaptureEvent(
            type: .audioChunk,
            bundleID: "us.zoom.xos",
            sessionID: "session",
            source: .remote,
            path: "/tmp/remote-1-0.wav",
            startMS: 0,
            endMS: 1_000,
            sampleRate: 16_000,
            channels: 1
        )

        let data = try JSONLineEncoder.encode(event)
        let object = try XCTUnwrap(
            JSONSerialization.jsonObject(with: data) as? [String: Any]
        )
        XCTAssertEqual(object["type"] as? String, "audio_chunk")
        XCTAssertEqual(object["bundle_id"] as? String, "us.zoom.xos")
        XCTAssertEqual(object["start_ms"] as? Int, 0)
        XCTAssertEqual(object["sample_rate"] as? Int, 16_000)
        XCTAssertNil(object["startMS"])
        XCTAssertFalse(String(decoding: data, as: UTF8.self).contains("null"))
    }

    func testFrameHashAndChangeGateAreDeterministic() {
        let first = Data(repeating: 0x10, count: 64)
        let second = Data(repeating: 0x11, count: 64)
        XCTAssertEqual(FrameHasher.hash(data: first), FrameHasher.hash(data: first))
        XCTAssertNotEqual(FrameHasher.hash(data: first), FrameHasher.hash(data: second))

        var gate = FrameChangeGate()
        XCTAssertTrue(gate.shouldProcess(FrameHasher.hash(data: first)))
        XCTAssertFalse(gate.shouldProcess(FrameHasher.hash(data: first)))
        XCTAssertTrue(gate.shouldProcess(FrameHasher.hash(data: second)))

        var rateGate = OCRChangeGate()
        XCTAssertTrue(rateGate.shouldProcess(hash: 1, timestampMS: 0))
        XCTAssertFalse(rateGate.shouldProcess(hash: 2, timestampMS: 999))
        XCTAssertTrue(rateGate.shouldProcess(hash: 2, timestampMS: 1_000))
    }

    func testChunkRotationFlushesTimestampedPCMFiles() throws {
        let directory = try temporaryDirectory()
        let writer = try AudioChunkWriter(
            source: .remote,
            spoolDirectory: directory,
            chunkSeconds: 1
        )
        let frameCount = 20_000
        let pcm = Data(repeating: 0, count: frameCount * WAVWriter.bytesPerFrame)
        let completed = try writer.append(pcm, frameCount: frameCount, timestampMS: 250)

        XCTAssertEqual(completed.count, 1)
        XCTAssertEqual(completed[0].startMS, 250)
        XCTAssertEqual(completed[0].endMS, 1_250)
        XCTAssertEqual(completed[0].sampleRate, 16_000)
        XCTAssertEqual(completed[0].channels, 1)
        XCTAssertEqual(try Data(contentsOf: completed[0].path).count, 44 + 32_000)

        let flushed = try writer.flush()
        XCTAssertEqual(flushed.count, 1)
        XCTAssertEqual(flushed[0].startMS, 1_250)
        XCTAssertEqual(flushed[0].endMS, 1_500)
        XCTAssertEqual(try Data(contentsOf: flushed[0].path).count, 44 + 8_000)
    }

    private func temporaryDirectory() throws -> URL {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("call-analyzer-capture-tests")
            .appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(
            at: directory,
            withIntermediateDirectories: true
        )
        addTeardownBlock {
            try? FileManager.default.removeItem(at: directory)
        }
        return directory
    }

    private func readUInt16(_ data: Data, at offset: Int) -> UInt16 {
        UInt16(data[offset]) | (UInt16(data[offset + 1]) << 8)
    }

    private func readUInt32(_ data: Data, at offset: Int) -> UInt32 {
        UInt32(data[offset]) |
            (UInt32(data[offset + 1]) << 8) |
            (UInt32(data[offset + 2]) << 16) |
            (UInt32(data[offset + 3]) << 24)
    }
}
