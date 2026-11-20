import AppKit
import ApplicationServices
import Carbon.HIToolbox
import Foundation

@MainActor
final class LookupAgent: NSObject, NSApplicationDelegate {
    private static let accessibilityNotice =
        "Allow Accessibility for call-analyzer-lookup in System Settings → Privacy & Security → Accessibility, then press ⌘⌥5 again."
    private static let emptySelectionNotice =
        "Highlight a Chinese word, then press ⌘⌥5."
    private static let registrationNotice = "⌘⌥5 is used by another app."

    private let writer = JSONLineWriter()
    private let decoder = JSONDecoder()
    private let panel = PanelController()
    private lazy var reader = JSONLineReader(
        onLine: { [weak self] data in
            DispatchQueue.main.async { [weak self] in
                self?.receive(data)
            }
        },
        onClose: { [weak self] in
            DispatchQueue.main.async { [weak self] in
                self?.stdinClosed()
            }
        }
    )

    private enum RequestKind {
        case lookup
        case add
        case regenerate
        case save
    }

    private struct LastLookupResult {
        let surface: String
        let lines: [GlossLine]
        let notice: String
        let existingTags: [String]
    }

    private var requestCounter: UInt64 = 0
    private var currentRequestID: String?
    private var currentAnchor: PanelAnchor?
    private var currentSurface = ""
    private var activeRequestKind: RequestKind?
    private var lastLookupSelection: SelectionRead?
    private var lastLookupResult: LastLookupResult?
    private var lastAddLine: GlossLine?
    private var preservedRegenerateTags: [String]?
    private var hotKey: EventHotKeyRef?
    private var eventHandler: EventHandlerRef?

    override init() {
        super.init()
        panel.onDismiss = { [weak self] in
            self?.dismiss()
        }
        panel.onAdd = { [weak self] line in
            self?.add(line)
        }
        panel.onSave = { [weak self] fields in
            self?.save(fields)
        }
        panel.onRetryAdd = { [weak self] in
            self?.retryAdd()
        }
        panel.onRetryLookup = { [weak self] in
            self?.retryLookup()
        }
        panel.onBackToResults = { [weak self] in
            self?.backToResults()
        }
        panel.onRegenerate = { [weak self] fields, instruction in
            self?.regenerate(fields: fields, instruction: instruction)
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        _ = NSApp.setActivationPolicy(.accessory)
        registerHotKey()
        reader.start()
    }

    func applicationWillTerminate(_ notification: Notification) {
        if let hotKey {
            UnregisterEventHotKey(hotKey)
            self.hotKey = nil
        }
        if let eventHandler {
            RemoveEventHandler(eventHandler)
            self.eventHandler = nil
        }
    }

    private func hotKeyPressed() {
        let mouseLocation = NSEvent.mouseLocation
        let fallbackAnchor = PanelAnchor(
            selectionRect: nil,
            mouseLocation: mouseLocation
        )
        currentAnchor = fallbackAnchor
        activeRequestKind = nil
        lastLookupSelection = nil
        lastLookupResult = nil
        lastAddLine = nil
        preservedRegenerateTags = nil

        guard AXIsProcessTrusted() else {
            _ = nextRequestID()
            promptForAccessibility()
            panel.showNotice(Self.accessibilityNotice, anchor: fallbackAnchor)
            return
        }

        guard let selection = readSelection() else {
            _ = nextRequestID()
            panel.showNotice(Self.emptySelectionNotice, anchor: fallbackAnchor)
            return
        }

        currentSurface = selection.surface
        lastLookupSelection = selection
        let anchor = PanelAnchor(
            selectionRect: selection.bounds,
            mouseLocation: mouseLocation
        )
        currentAnchor = anchor
        sendLookup(selection, anchor: anchor)
    }

    private func sendLookup(_ selection: SelectionRead, anchor: PanelAnchor) {
        let id = nextRequestID()
        activeRequestKind = .lookup
        panel.showLoading(surface: selection.surface, anchor: anchor)

        let bounds = selection.bounds.map {
            SelectionBounds(
                x: $0.origin.x,
                y: $0.origin.y,
                width: $0.width,
                height: $0.height
            )
        }
        writer.write(LookupRequest(
            id: id,
            surface: selection.surface,
            context: selection.context,
            bounds: bounds
        ))
    }

    private func receive(_ data: Data) {
        guard let message = try? decoder.decode(ParentMessage.self, from: data),
              !message.id.isEmpty,
              message.id == currentRequestID,
              let anchor = currentAnchor
        else {
            return
        }

        switch message.type {
        case "result":
            activeRequestKind = nil
            let surface = message.surface ?? currentSurface
            let lines = message.lines ?? []
            let notice = message.notice ?? ""
            let existingTags = message.existingTags ?? []
            lastLookupResult = LastLookupResult(
                surface: surface,
                lines: lines,
                notice: notice,
                existingTags: existingTags
            )
            panel.showResult(
                surface: surface,
                lines: lines,
                notice: notice,
                existingTags: existingTags,
                anchor: anchor
            )
        case "draft":
            let wasRegenerating = activeRequestKind == .regenerate
            activeRequestKind = nil
            if var draft = message.draft {
                if wasRegenerating,
                   let preservedTags = preservedRegenerateTags,
                   !preservedTags.isEmpty,
                   (draft.fields.tags ?? []).isEmpty {
                    draft = CardDraft(
                        fields: CardFields(
                            simplified: draft.fields.simplified,
                            traditional: draft.fields.traditional,
                            pinyin: draft.fields.pinyin,
                            meaning: draft.fields.meaning,
                            partOfSpeech: draft.fields.partOfSpeech,
                            classifier: draft.fields.classifier,
                            example: draft.fields.example,
                            examplePinyin: draft.fields.examplePinyin,
                            exampleTranslation: draft.fields.exampleTranslation,
                            tags: preservedTags,
                            aiGenerated: draft.fields.aiGenerated
                        ),
                        needsChoice: draft.needsChoice,
                        readOnly: draft.readOnly,
                        variants: draft.variants
                    )
                }
                preservedRegenerateTags = nil
                panel.showCard(
                    draft,
                    notice: wasRegenerating ? "Card regenerated. Review before saving." : nil,
                    anchor: anchor
                )
            }
        case "saved":
            activeRequestKind = nil
            preservedRegenerateTags = nil
            panel.showSaved(anchor: anchor)
        case "error":
            let kind = activeRequestKind
            activeRequestKind = nil
            preservedRegenerateTags = nil
            let headline = nonEmpty(message.message) ?? defaultErrorTitle(for: kind)
            let detail = nonEmpty(message.detail)
            switch kind {
            case .save:
                if panel.shouldKeepForm() {
                    panel.showFormError(title: headline, detail: detail)
                } else {
                    panel.showError(title: headline, detail: detail, anchor: anchor)
                }
            case .regenerate:
                if panel.shouldKeepForm() {
                    panel.showFormError(
                        title: "Couldn't regenerate this card.",
                        detail: detail ?? (headline == "Couldn't add this word." ? nil : headline)
                    )
                } else {
                    panel.showAddError(
                        line: lastAddLine,
                        title: headline,
                        detail: detail,
                        canGoBack: lastLookupResult != nil,
                        anchor: anchor
                    )
                }
            case .add:
                panel.showAddError(
                    line: lastAddLine,
                    title: headline,
                    detail: detail,
                    canGoBack: lastLookupResult != nil,
                    anchor: anchor
                )
            case .lookup:
                panel.showLookupError(
                    surface: currentSurface,
                    title: headline,
                    detail: detail,
                    canRetry: lastLookupSelection != nil,
                    anchor: anchor
                )
            case .none:
                if panel.shouldKeepForm() {
                    panel.showFormError(title: headline, detail: detail)
                } else {
                    panel.showError(title: headline, detail: detail, anchor: anchor)
                }
            }
        case "dismiss":
            dismiss()
        default:
            break
        }
    }

    private func add(_ line: GlossLine) {
        guard let anchor = currentAnchor else {
            return
        }
        if line.action == "open" {
            activeRequestKind = nil
            panel.showCard(savedCard(from: line), anchor: anchor)
            return
        }
        lastAddLine = line
        let id = nextRequestID()
        activeRequestKind = .add
        panel.showAdding(line: line, anchor: anchor)
        writer.write(AddRequest(id: id, line: line))
    }

    private func retryAdd() {
        guard let line = lastAddLine, let anchor = currentAnchor else {
            return
        }
        let id = nextRequestID()
        activeRequestKind = .add
        panel.showAdding(line: line, anchor: anchor)
        writer.write(AddRequest(id: id, line: line))
    }

    private func retryLookup() {
        guard let selection = lastLookupSelection, let anchor = currentAnchor else {
            return
        }
        sendLookup(selection, anchor: anchor)
    }

    private func backToResults() {
        guard let result = lastLookupResult, let anchor = currentAnchor else {
            return
        }
        _ = nextRequestID()
        activeRequestKind = nil
        preservedRegenerateTags = nil
        panel.showResult(
            surface: result.surface,
            lines: result.lines,
            notice: result.notice,
            existingTags: result.existingTags,
            anchor: anchor
        )
    }

    private func regenerate(fields: CardFields, instruction: String) {
        let trimmed = instruction.trimmingCharacters(in: .whitespacesAndNewlines)
        preservedRegenerateTags = fields.tags
        let id = nextRequestID()
        activeRequestKind = .regenerate
        panel.setCardBusy(message: "Regenerating…")
        writer.write(RegenerateRequest(
            id: id,
            fields: fields,
            instruction: trimmed.isEmpty ? nil : trimmed
        ))
    }

    private func save(_ fields: CardFields) {
        let id = nextRequestID()
        activeRequestKind = .save
        panel.setCardBusy(message: "Saving…")
        writer.write(SaveRequest(id: id, fields: fields))
    }

    private func defaultErrorTitle(for kind: RequestKind?) -> String {
        switch kind {
        case .lookup:
            return "Couldn't look this up."
        case .add:
            return "Couldn't add this word."
        case .regenerate:
            return "Couldn't regenerate this card."
        case .save:
            return "Couldn't save this word."
        case .none:
            return "Something went wrong."
        }
    }

    private func nonEmpty(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty
        else {
            return nil
        }
        return trimmed
    }

    private func savedCard(from line: GlossLine) -> CardDraft {
        CardDraft(
            fields: CardFields(
                simplified: line.simplified,
                traditional: line.traditional,
                pinyin: line.pinyin,
                meaning: line.meaning == "No English gloss saved" ? "" : line.meaning,
                partOfSpeech: line.partOfSpeech,
                classifier: line.classifier,
                example: line.example,
                examplePinyin: line.examplePinyin,
                exampleTranslation: line.exampleTranslation,
                tags: line.tags,
                aiGenerated: false
            ),
            needsChoice: false,
            readOnly: true,
            variants: nil
        )
    }

    private func dismiss() {
        currentRequestID = nil
        currentAnchor = nil
        currentSurface = ""
        activeRequestKind = nil
        lastLookupSelection = nil
        lastLookupResult = nil
        lastAddLine = nil
        preservedRegenerateTags = nil
        panel.hide()
    }

    private func nextRequestID() -> String {
        requestCounter &+= 1
        let id = String(requestCounter)
        currentRequestID = id
        return id
    }

    private func stdinClosed() {
        panel.hide()
        NSApp.terminate(nil)
    }

    private func promptForAccessibility() {
        let options = [
            "AXTrustedCheckOptionPrompt": true
        ] as CFDictionary
        _ = AXIsProcessTrustedWithOptions(options)
    }

    private func registerHotKey() {
        var eventType = EventTypeSpec(
            eventClass: OSType(kEventClassKeyboard),
            eventKind: UInt32(kEventHotKeyPressed)
        )
        var installedHandler: EventHandlerRef?
        let userData = Unmanaged.passUnretained(self).toOpaque()
        let installStatus = InstallEventHandler(
            GetApplicationEventTarget(),
            { _, _, userData in
                guard let userData else {
                    return noErr
                }
                let pointer = userData
                DispatchQueue.main.async {
                    Unmanaged<LookupAgent>
                        .fromOpaque(pointer)
                        .takeUnretainedValue()
                        .hotKeyPressed()
                }
                return noErr
            },
            1,
            &eventType,
            userData,
            &installedHandler
        )
        guard installStatus == noErr else {
            showRegistrationFailure()
            return
        }
        eventHandler = installedHandler

        let hotKeyID = EventHotKeyID(signature: 0x43414C4C, id: 1)
        var registeredHotKey: EventHotKeyRef?
        let registerStatus = RegisterEventHotKey(
            UInt32(kVK_ANSI_5),
            UInt32(cmdKey | optionKey),
            hotKeyID,
            GetApplicationEventTarget(),
            0,
            &registeredHotKey
        )
        guard registerStatus == noErr else {
            if let installedHandler {
                RemoveEventHandler(installedHandler)
                eventHandler = nil
            }
            showRegistrationFailure()
            return
        }
        hotKey = registeredHotKey
    }

    private func showRegistrationFailure() {
        let anchor = PanelAnchor(
            selectionRect: nil,
            mouseLocation: NSEvent.mouseLocation
        )
        panel.showNotice(Self.registrationNotice, anchor: anchor)
    }

    private struct SelectionRead {
        let surface: String
        let context: String?
        let bounds: CGRect?
    }

    private func readSelection() -> SelectionRead? {
        let systemWide = AXUIElementCreateSystemWide()
        guard let focusedApplication = copyAttribute(
            from: systemWide,
            name: kAXFocusedApplicationAttribute
        ),
        let application = axElement(from: focusedApplication),
        let focusedValue = copyAttribute(
            from: application,
            name: kAXFocusedUIElementAttribute
        ),
        let focusedElement = axElement(from: focusedValue),
        let selectedValue = copyAttribute(
            from: focusedElement,
            name: kAXSelectedTextAttribute
        ),
        let surface = selectedValue as? String,
        !surface.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty
        else {
            return nil
        }

        guard let selectedRangeValue = copyAttribute(
            from: focusedElement,
            name: kAXSelectedTextRangeAttribute
        ),
        let selectedRange = cfRange(from: selectedRangeValue),
        selectedRange.location >= 0,
        selectedRange.length > 0
        else {
            return SelectionRead(surface: surface, context: nil, bounds: nil)
        }

        let context = surroundingText(
            from: focusedElement,
            range: selectedRange
        )
        let bounds = bounds(
            from: focusedElement,
            range: selectedRange
        )
        return SelectionRead(surface: surface, context: context, bounds: bounds)
    }

    private func axElement(from value: CFTypeRef) -> AXUIElement? {
        guard CFGetTypeID(value) == AXUIElementGetTypeID() else {
            return nil
        }
        return (value as! AXUIElement)
    }

    private func copyAttribute(
        from element: AXUIElement,
        name: String
    ) -> CFTypeRef? {
        var value: CFTypeRef?
        let status = AXUIElementCopyAttributeValue(element, name as CFString, &value)
        guard status == .success else {
            return nil
        }
        return value
    }

    private func copyParameterizedAttribute(
        from element: AXUIElement,
        name: String,
        parameter: CFTypeRef
    ) -> CFTypeRef? {
        var value: CFTypeRef?
        let status = AXUIElementCopyParameterizedAttributeValue(
            element,
            name as CFString,
            parameter,
            &value
        )
        guard status == .success else {
            return nil
        }
        return value
    }

    private func cfRange(from value: CFTypeRef) -> CFRange? {
        guard CFGetTypeID(value) == AXValueGetTypeID() else {
            return nil
        }
        let axValue = value as! AXValue
        guard AXValueGetType(axValue) == .cfRange else {
            return nil
        }
        var range = CFRange()
        guard AXValueGetValue(axValue, .cfRange, &range) else {
            return nil
        }
        return range
    }

    private func surroundingText(
        from element: AXUIElement,
        range: CFRange
    ) -> String? {
        var surroundingRange = CFRange(
            location: max(0, range.location - 40),
            length: range.length + 80
        )
        if let rangeValue = AXValueCreate(.cfRange, &surroundingRange),
           let value = copyParameterizedAttribute(
               from: element,
               name: kAXStringForRangeParameterizedAttribute,
               parameter: rangeValue
           ) as? String,
           !value.isEmpty {
            return value
        }

        guard let fullValue = copyAttribute(
            from: element,
            name: kAXValueAttribute
        ) as? String
        else {
            return nil
        }
        return crop(fullValue, around: range)
    }

    private func crop(_ value: String, around range: CFRange) -> String? {
        let text = value as NSString
        guard range.location >= 0,
              range.length >= 0,
              range.location <= text.length
        else {
            return nil
        }
        let start = max(0, range.location - 40)
        let end = min(text.length, range.location + range.length + 40)
        guard end >= start else {
            return nil
        }
        return text.substring(with: NSRange(location: start, length: end - start))
    }

    private func bounds(
        from element: AXUIElement,
        range: CFRange
    ) -> CGRect? {
        var mutableRange = range
        guard let rangeValue = AXValueCreate(.cfRange, &mutableRange),
              let value = copyParameterizedAttribute(
                  from: element,
                  name: kAXBoundsForRangeParameterizedAttribute,
                  parameter: rangeValue
              )
        else {
            return nil
        }

        guard CFGetTypeID(value) == AXValueGetTypeID() else {
            return nil
        }
        let axValue = value as! AXValue
        guard AXValueGetType(axValue) == .cgRect else {
            return nil
        }
        var rect = CGRect.zero
        guard AXValueGetValue(axValue, .cgRect, &rect),
              rect.width > 0,
              rect.height > 0
        else {
            return nil
        }
        return rect
    }
}
