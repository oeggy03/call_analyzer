import CoreMedia
import CoreVideo
import Foundation
import Vision

struct OCRResult: Equatable {
    let text: String
    let timestampMS: Int64
    let confidence: Double
}

final class OCRProcessor {
    private let requestedLanguages = ["zh-Hans", "zh-Hant", "en-US"]
    private var changeGate = OCRChangeGate()
    private var lastText: String?

    func recognize(
        pixelBuffer: CVPixelBuffer,
        timestampMS: Int64
    ) throws -> OCRResult? {
        let hash = FrameHasher.hash(pixelBuffer: pixelBuffer)
        guard changeGate.shouldProcess(hash: hash, timestampMS: timestampMS) else {
            return nil
        }

        let request = VNRecognizeTextRequest()
        request.recognitionLevel = .fast
        request.recognitionLanguages = requestedLanguages
        // Keep raw Vision output. No custom lexicon or language-correction support
        // is claimed by this adapter.
        request.usesLanguageCorrection = false
        request.minimumTextHeight = 0.01

        let handler = VNImageRequestHandler(cvPixelBuffer: pixelBuffer, options: [:])
        try handler.perform([request])

        let observations = request.results ?? []
        var lines: [String] = []
        var confidenceTotal = 0.0
        var confidenceCount = 0
        for observation in observations {
            guard let candidate = observation.topCandidates(1).first else {
                continue
            }
            let line = candidate.string.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !line.isEmpty else {
                continue
            }
            lines.append(line)
            confidenceTotal += Double(candidate.confidence)
            confidenceCount += 1
        }

        let text = lines.joined(separator: "\n")
        guard !text.isEmpty, text != lastText else {
            return nil
        }
        lastText = text
        return OCRResult(
            text: text,
            timestampMS: timestampMS,
            confidence: confidenceCount == 0
                ? 0
                : confidenceTotal / Double(confidenceCount)
        )
    }
}
