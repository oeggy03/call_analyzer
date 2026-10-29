import Darwin
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
            // FileHandle.readData(ofLength:) treats a not-yet-ready pipe as
            // end of input, so a lookup reply never reaches the panel.
            let fd = STDIN_FILENO
            let flags = fcntl(fd, F_GETFL)
            if flags >= 0 {
                _ = fcntl(fd, F_SETFL, flags & ~O_NONBLOCK)
            }
            var buffer = Data()

            while true {
                var chunk = [UInt8](repeating: 0, count: 4096)
                let count = Darwin.read(fd, &chunk, chunk.count)
                if count == 0 {
                    break
                }
                if count < 0 {
                    if errno == EINTR {
                        continue
                    }
                    break
                }
                buffer.append(contentsOf: chunk[0..<count])

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
