import Foundation

final class JSONLineWriter: @unchecked Sendable {
    private let lock = NSLock()
    private let encoder = JSONEncoder()

    func write(_ value: any Encodable) {
        guard let encoded = try? encoder.encode(value) else {
            return
        }
        var data = encoded
        data.append(0x0A)

        lock.lock()
        defer { lock.unlock() }
        try? FileHandle.standardOutput.write(contentsOf: data)
    }
}

final class JSONLineReader: @unchecked Sendable {
    private let onLine: @Sendable (Data) -> Void
    private let onClose: @Sendable () -> Void

    init(
        onLine: @escaping @Sendable (Data) -> Void,
        onClose: @escaping @Sendable () -> Void
    ) {
        self.onLine = onLine
        self.onClose = onClose
    }

    func start() {
        let onLine = self.onLine
        let onClose = self.onClose
        DispatchQueue.global(qos: .utility).async {
            let input = FileHandle.standardInput
            var buffer = Data()

            while true {
                let chunk = input.readData(ofLength: 4096)
                if chunk.isEmpty {
                    break
                }
                buffer.append(chunk)

                while let newline = buffer.firstIndex(of: 0x0A) {
                    var line = Data(buffer[..<newline])
                    buffer.removeSubrange(buffer.startIndex...newline)
                    if line.last == 0x0D {
                        line.removeLast()
                    }
                    onLine(line)
                }
            }
            onClose()
        }
    }
}
