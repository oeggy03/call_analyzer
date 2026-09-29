import Darwin
import Foundation

private enum CLICommand {
    case list
    case capture(CaptureOptions)
    case stop(pid_t)
}

private struct CLI {
    static func parse(_ arguments: [String]) throws -> CLICommand {
        guard let command = arguments.first else {
            throw CaptureFailure.invalidArguments(usage)
        }
        switch command {
        case "list":
            guard arguments.count == 1 else {
                throw CaptureFailure.invalidArguments("list takes no options.\n\(usage)")
            }
            return .list
        case "capture":
            return .capture(try parseCapture(Array(arguments.dropFirst())))
        case "stop":
            return .stop(try parsePID(Array(arguments.dropFirst())))
        case "--help", "-h", "help":
            throw CaptureFailure.invalidArguments(usage)
        default:
            throw CaptureFailure.invalidArguments(
                "Unknown command '\(command)'.\n\(usage)"
            )
        }
    }

    private static func parseCapture(_ arguments: [String]) throws -> CaptureOptions {
        var bundleID: String?
        var spoolDirectory: URL?
        var microphone = false
        var ocr = false
        var chunkSeconds = 10

        var index = 0
        while index < arguments.count {
            let argument = arguments[index]
            switch argument {
            case "--bundle-id":
                bundleID = try value(for: argument, arguments: arguments, index: &index)
            case "--spool":
                spoolDirectory = URL(
                    fileURLWithPath: try value(
                        for: argument,
                        arguments: arguments,
                        index: &index
                    ),
                    isDirectory: true
                )
            case "--microphone":
                microphone = true
            case "--ocr":
                ocr = true
            case "--chunk-seconds":
                let value = try value(for: argument, arguments: arguments, index: &index)
                guard let parsed = Int(value), (1...3600).contains(parsed) else {
                    throw CaptureFailure.invalidArguments(
                        "--chunk-seconds must be an integer between 1 and 3600"
                    )
                }
                chunkSeconds = parsed
            default:
                throw CaptureFailure.invalidArguments(
                    "Unknown capture option '\(argument)'.\n\(usage)"
                )
            }
            index += 1
        }

        guard
            let bundleID,
            !bundleID.isEmpty,
            let spoolDirectory
        else {
            throw CaptureFailure.invalidArguments(
                "capture requires --bundle-id and --spool.\n\(usage)"
            )
        }
        return CaptureOptions(
            bundleID: bundleID,
            spoolDirectory: spoolDirectory,
            microphone: microphone,
            ocr: ocr,
            chunkSeconds: chunkSeconds
        )
    }

    private static func parsePID(_ arguments: [String]) throws -> pid_t {
        guard arguments.count == 2, arguments[0] == "--pid",
              let value = Int32(arguments[1]), value > 0
        else {
            throw CaptureFailure.invalidArguments(
                "stop requires --pid <capture-process-id>.\n\(usage)"
            )
        }
        return pid_t(value)
    }

    private static func value(
        for option: String,
        arguments: [String],
        index: inout Int
    ) throws -> String {
        let valueIndex = index + 1
        guard valueIndex < arguments.count, !arguments[valueIndex].isEmpty else {
            throw CaptureFailure.invalidArguments("\(option) requires a value")
        }
        index = valueIndex
        return arguments[valueIndex]
    }

    private static let usage = """
    Usage:
      call-analyzer-capture list
      call-analyzer-capture capture --bundle-id <id> --spool <dir> [--microphone] [--ocr] [--chunk-seconds 10]
      call-analyzer-capture stop --pid <capture-process-id>
    """
}

private final class SignalHandler {
    private var sources: [DispatchSourceSignal] = []

    init(onSignal: @escaping () -> Void) {
        for signalNumber in [SIGINT, SIGTERM] {
            Darwin.signal(signalNumber, SIG_IGN)
            let source = DispatchSource.makeSignalSource(
                signal: signalNumber,
                queue: DispatchQueue.global(qos: .userInitiated)
            )
            source.setEventHandler(handler: onSignal)
            source.resume()
            sources.append(source)
        }
    }

    deinit {
        for source in sources {
            source.cancel()
        }
    }
}

@main
struct CaptureCLI {
    static func main() async {
        let sink = EventSink()
        do {
            switch try CLI.parse(Array(CommandLine.arguments.dropFirst())) {
            case .list:
                let content = try await ShareableContentCatalog.current()
                for target in ShareableContentCatalog.targetEvents(from: content) {
                    sink.emit(target)
                }
            case .capture(let options):
                let engine = CaptureEngine(options: options, sink: sink)
                do {
                    try await engine.start()
                } catch {
                    emit(error: error, sink: sink)
                    Darwin.exit(1)
                }
                let signalHandler = SignalHandler {
                    engine.requestStop()
                }
                await engine.waitForStopRequest()
                _ = signalHandler
                await engine.stop()
            case .stop(let pid):
                guard Darwin.kill(pid, SIGTERM) == 0 else {
                    let message = String(
                        cString: strerror(errno)
                    )
                    throw CaptureFailure.stream(
                        "could not signal capture process \(pid): \(message)"
                    )
                }
                sink.emit(
                    CaptureEvent(
                        type: .state,
                        state: "stop_requested",
                        message: "SIGTERM sent to capture process \(pid)"
                    )
                )
            }
        } catch {
            emit(error: error, sink: sink)
            Darwin.exit(1)
        }
        Darwin.exit(0)
    }

    private static func emit(error: Error, sink: EventSink) {
        if let failure = error as? CaptureFailure {
            sink.error(
                code: failure.code,
                message: failure.localizedDescription,
                actionable: failure.actionable
            )
        } else {
            sink.error(code: "capture_error", message: error.localizedDescription)
        }
    }
}
