import AVFoundation
import Foundation
import ScreenCaptureKit

enum CaptureFailure: Error, LocalizedError {
    case invalidArguments(String)
    case screenRecordingPermission(String)
    case microphonePermission(String)
    case bundleNotFound(String)
    case noDisplay
    case stream(String)
    case unsupportedPlatform(String)

    var code: String {
        switch self {
        case .invalidArguments:
            return "invalid_arguments"
        case .screenRecordingPermission:
            return "screen_recording_permission"
        case .microphonePermission:
            return "microphone_permission"
        case .bundleNotFound:
            return "bundle_not_found"
        case .noDisplay:
            return "no_display"
        case .stream:
            return "stream_error"
        case .unsupportedPlatform:
            return "unsupported_platform"
        }
    }

    var actionable: Bool {
        switch self {
        case .invalidArguments, .bundleNotFound, .noDisplay:
            return true
        case .screenRecordingPermission, .microphonePermission:
            return true
        case .stream, .unsupportedPlatform:
            return false
        }
    }

    var errorDescription: String? {
        switch self {
        case .invalidArguments(let message),
             .screenRecordingPermission(let message),
             .microphonePermission(let message),
             .bundleNotFound(let message),
             .stream(let message),
             .unsupportedPlatform(let message):
            return message
        case .noDisplay:
            return "No capturable display is available. Keep at least one display connected and retry."
        }
    }
}

struct ShareableContentCatalog {
    static func current() async throws -> SCShareableContent {
        do {
            return try await SCShareableContent.excludingDesktopWindows(
                true,
                onScreenWindowsOnly: false
            )
        } catch {
            throw CaptureFailure.screenRecordingPermission(
                "Screen Recording permission is required. Open System Settings > Privacy & Security > Screen Recording, allow this helper, then retry. \(error.localizedDescription)"
            )
        }
    }

    static func targetEvents(from content: SCShareableContent) -> [CaptureEvent] {
        content.applications
            .sorted {
                if $0.applicationName == $1.applicationName {
                    return $0.bundleIdentifier < $1.bundleIdentifier
                }
                return $0.applicationName < $1.applicationName
            }
            .map { application in
                let windows = content.windows
                    .filter { $0.owningApplication?.processID == application.processID }
                    .sorted { $0.windowID < $1.windowID }
                    .map { window in
                        CaptureWindow(
                            windowID: window.windowID,
                            title: window.title ?? "",
                            onScreen: window.isOnScreen,
                            active: window.isActive,
                            x: Double(window.frame.origin.x),
                            y: Double(window.frame.origin.y),
                            width: Double(window.frame.size.width),
                            height: Double(window.frame.size.height)
                        )
                    }
                return CaptureEvent(
                    type: .target,
                    bundleID: application.bundleIdentifier,
                    applicationName: application.applicationName,
                    processID: application.processID,
                    windows: windows
                )
            }
    }

    static func application(
        bundleID: String,
        in content: SCShareableContent
    ) throws -> SCRunningApplication {
        guard let application = content.applications.first(where: {
            $0.bundleIdentifier == bundleID
        }) else {
            throw CaptureFailure.bundleNotFound(
                "No capturable running application matched bundle id '\(bundleID)'. Run list first and verify the app is running."
            )
        }
        return application
    }

    static func filter(
        for application: SCRunningApplication,
        in content: SCShareableContent
    ) throws -> SCContentFilter {
        guard let display = content.displays.first else {
            throw CaptureFailure.noDisplay
        }
        return SCContentFilter(
            display: display,
            including: [application],
            exceptingWindows: []
        )
    }

    static func ensureMicrophonePermission() async throws {
        switch AVCaptureDevice.authorizationStatus(for: .audio) {
        case .authorized:
            return
        case .notDetermined:
            let granted = await withCheckedContinuation { continuation in
                AVCaptureDevice.requestAccess(for: .audio) { allowed in
                    continuation.resume(returning: allowed)
                }
            }
            guard granted else {
                throw CaptureFailure.microphonePermission(
                    "Microphone permission was denied. Open System Settings > Privacy & Security > Microphone and allow this helper, then retry."
                )
            }
        case .denied, .restricted:
            throw CaptureFailure.microphonePermission(
                "Microphone permission is unavailable. Open System Settings > Privacy & Security > Microphone and allow this helper, then retry."
            )
        @unknown default:
            throw CaptureFailure.microphonePermission(
                "The microphone authorization state is unknown; check System Settings > Privacy & Security > Microphone."
            )
        }
    }
}
