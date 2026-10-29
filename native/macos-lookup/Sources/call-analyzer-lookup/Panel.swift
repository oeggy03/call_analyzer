import AppKit

private extension NSColor {
    static let lookupInk = NSColor(calibratedRed: 0x15 / 255.0, green: 0x20 / 255.0, blue: 0x33 / 255.0, alpha: 1)
    static let lookupSecondary = NSColor(calibratedRed: 0x4B / 255.0, green: 0x5A / 255.0, blue: 0x70 / 255.0, alpha: 1)
    static let lookupNavy = NSColor(calibratedRed: 0x20 / 255.0, green: 0x37 / 255.0, blue: 0x5E / 255.0, alpha: 1)
    static let lookupLine = NSColor(calibratedRed: 0xE2 / 255.0, green: 0xE7 / 255.0, blue: 0xEE / 255.0, alpha: 1)
    static let lookupButton = NSColor(calibratedRed: 0x39 / 255.0, green: 0x74 / 255.0, blue: 0xC8 / 255.0, alpha: 1)
}

private final class LookupPanel: NSPanel {
    override var canBecomeKey: Bool {
        true
    }

    override var canBecomeMain: Bool {
        false
    }
}

private final class PaddedButton: NSButton {
    override var intrinsicContentSize: NSSize {
        let size = super.intrinsicContentSize
        guard size.width != NSView.noIntrinsicMetric else {
            return size
        }
        return NSSize(width: size.width + 24, height: size.height)
    }

    override func resetCursorRects() {
        addCursorRect(bounds, cursor: .pointingHand)
    }
}

struct PanelAnchor {
    let selectionRect: CGRect?
    let mouseLocation: NSPoint
}

@MainActor
final class PanelController: NSObject {
    var onDismiss: (() -> Void)?
    var onAdd: ((GlossLine) -> Void)?
    var onSave: ((CardFields) -> Void)?

    private let panel: LookupPanel
    private let bodyStack = NSStackView()
    private let closeButton: PaddedButton
    private let panelWidth: CGFloat = 340
    private var currentLines: [GlossLine] = []
    private var fieldInputs: [String: NSTextField] = [:]
    private var formErrorLabel: NSTextField?
    private var generatedCard = false
    private var showingCard = false
    private var cardVariants: [CardVariant] = []
    private var lastAnchor: PanelAnchor?

    override init() {
        panel = LookupPanel(
            contentRect: NSRect(x: 0, y: 0, width: 280, height: 120),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: true
        )
        closeButton = PaddedButton(title: "Close", target: nil, action: nil)
        super.init()
        configurePanel()
    }

    func showLoading(surface: String, anchor: PanelAnchor) {
        currentLines = []
        replaceBody(with: [
            makeLabel(
                surface,
                font: chineseFont(size: 14, weight: .medium),
                color: .lookupInk
            ),
            makeLabel(
                "Looking up…",
                font: chineseFont(size: 12, weight: .regular),
                color: .lookupSecondary
            ),
        ])
        show(at: anchor)
    }

    func showResult(
        surface: String,
        lines: [GlossLine],
        notice: String,
        anchor: PanelAnchor
    ) {
        currentLines = Array(lines.prefix(3))
        if currentLines.isEmpty {
            replaceBody(with: [
                makeLabel(
                    surface,
                    font: chineseFont(size: 14, weight: .medium),
                    color: .lookupInk
                ),
                makeLabel(
                    notice,
                    font: chineseFont(size: 12, weight: .regular),
                    color: .lookupSecondary
                ),
            ])
        } else if currentLines.count == 1, let line = currentLines.first {
            replaceBody(with: makeSingleGlossViews(surface: surface, line: line))
        } else {
            replaceBody(with: makeSeveralGlossViews(surface: surface, lines: currentLines))
        }
        show(at: anchor)
    }

    func showNotice(_ message: String, anchor: PanelAnchor) {
        currentLines = []
        replaceBody(with: [
            makeLabel(
                message,
                font: chineseFont(size: 12, weight: .regular),
                color: .lookupSecondary
            )
        ])
        show(at: anchor)
    }

    func showError(_ message: String, anchor: PanelAnchor) {
        currentLines = []
        replaceBody(with: [
            makeLabel(
                message,
                font: chineseFont(size: 12, weight: .regular),
                color: .lookupSecondary
            )
        ])
        show(at: anchor)
    }

    func showAdding(action: String, anchor: PanelAnchor) {
        currentLines = []
        showingCard = false
        let message = action == "open" ? "Opening…" : "Adding…"
        replaceBody(with: [
            makeLabel(
                message,
                font: chineseFont(size: 12, weight: .regular),
                color: .lookupSecondary
            )
        ])
        show(at: anchor)
    }

    func hide() {
        panel.orderOut(nil)
        currentLines = []
        showingCard = false
        fieldInputs = [:]
    }

    func showCard(_ draft: CardDraft, anchor: PanelAnchor) {
        currentLines = []
        showingCard = true
        cardVariants = draft.variants ?? []
        generatedCard = draft.fields.aiGenerated == true
        fieldInputs = [:]
        var views: [NSView] = []
        if draft.readOnly {
            views.append(makeLabel(
                "Saved",
                font: chineseFont(size: 12, weight: .semibold),
                color: .lookupNavy
            ))
        } else if draft.needsChoice {
            views.append(makeLabel(
                "Choose a meaning",
                font: chineseFont(size: 12, weight: .semibold),
                color: .lookupNavy
            ))
            for (index, variant) in (draft.variants ?? []).enumerated() {
                views.append(makeChoiceButton(title: "\(variant.pinyin) · \(variant.meaning)", tag: index))
            }
        }
        views.append(contentsOf: makeCardFields(draft.fields, editable: !draft.readOnly))
        formErrorLabel = makeLabel(
            "",
            font: chineseFont(size: 12, weight: .regular),
            color: NSColor(calibratedRed: 0xBB / 255.0, green: 0x59 / 255.0, blue: 0x60 / 255.0, alpha: 1)
        )
        formErrorLabel?.isHidden = true
        if let formErrorLabel {
            views.append(formErrorLabel)
        }
        if !draft.readOnly {
            views.append(makeSaveButton())
        }
        replaceBody(with: views)
        show(at: anchor)
    }

    func shouldKeepForm() -> Bool {
        showingCard
    }

    func showFormStatus(_ message: String) {
        formErrorLabel?.stringValue = message
        formErrorLabel?.isHidden = message.isEmpty
        if let anchor = lastAnchor {
            show(at: anchor)
        }
    }

    func showSaved(anchor: PanelAnchor) {
        showingCard = false
        fieldInputs = [:]
        replaceBody(with: [
            makeLabel(
                "Saved to your vocabulary.",
                font: chineseFont(size: 13, weight: .semibold),
                color: .lookupInk
            ),
        ])
        show(at: anchor)
    }

    private func configurePanel() {
        panel.isFloatingPanel = true
        panel.level = .floating
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        panel.hidesOnDeactivate = false
        panel.becomesKeyOnlyIfNeeded = true
        panel.hasShadow = true
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.isReleasedWhenClosed = false

        guard let contentView = panel.contentView else {
            return
        }
        contentView.wantsLayer = true
        contentView.layer?.backgroundColor = NSColor.white.cgColor
        contentView.layer?.cornerRadius = 12
        contentView.layer?.masksToBounds = true

        let mainStack = NSStackView()
        mainStack.orientation = .vertical
        mainStack.alignment = .width
        mainStack.spacing = 10
        mainStack.translatesAutoresizingMaskIntoConstraints = false
        contentView.addSubview(mainStack)
        NSLayoutConstraint.activate([
            mainStack.leadingAnchor.constraint(equalTo: contentView.leadingAnchor, constant: 14),
            mainStack.trailingAnchor.constraint(equalTo: contentView.trailingAnchor, constant: -14),
            mainStack.topAnchor.constraint(equalTo: contentView.topAnchor, constant: 14),
            mainStack.bottomAnchor.constraint(equalTo: contentView.bottomAnchor, constant: -14),
        ])

        let closeRow = NSView()
        closeRow.translatesAutoresizingMaskIntoConstraints = false
        closeButton.target = self
        closeButton.action = #selector(closePressed)
        closeButton.isBordered = false
        closeButton.font = NSFont.systemFont(ofSize: 12, weight: .medium)
        closeButton.contentTintColor = .lookupSecondary
        closeButton.setAccessibilityLabel("Close")
        closeButton.translatesAutoresizingMaskIntoConstraints = false
        closeRow.addSubview(closeButton)
        NSLayoutConstraint.activate([
            closeRow.heightAnchor.constraint(equalToConstant: 20),
            closeButton.trailingAnchor.constraint(equalTo: closeRow.trailingAnchor),
            closeButton.topAnchor.constraint(equalTo: closeRow.topAnchor),
            closeButton.bottomAnchor.constraint(equalTo: closeRow.bottomAnchor),
        ])
        mainStack.addArrangedSubview(closeRow)

        bodyStack.orientation = .vertical
        bodyStack.alignment = .width
        bodyStack.spacing = 8
        mainStack.addArrangedSubview(bodyStack)
    }

    private func replaceBody(with views: [NSView]) {
        for view in bodyStack.arrangedSubviews {
            bodyStack.removeArrangedSubview(view)
            view.removeFromSuperview()
        }
        for view in views {
            bodyStack.addArrangedSubview(view)
        }
    }

    private func makeSingleGlossViews(surface: String, line: GlossLine) -> [NSView] {
        let word = makeLabel(
            surface,
            font: chineseFont(size: 14, weight: .medium),
            color: .lookupInk
        )
        let pinyin = makeLabel(
            line.pinyin,
                font: chineseFont(size: 15, weight: .semibold),
            color: .lookupNavy
        )
        let meaning = makeLabel(
            line.meaning,
            font: chineseFont(size: 12, weight: .regular),
            color: .lookupSecondary
        )
        let button = makeButton(
            title: buttonTitle(for: line),
            tag: 0,
            fullWidth: true
        )
        return [word, pinyin, meaning, button]
    }

    private func makeSeveralGlossViews(surface: String, lines: [GlossLine]) -> [NSView] {
        var views: [NSView] = [
            makeLabel(
                surface,
                font: chineseFont(size: 14, weight: .medium),
                color: .lookupInk
            )
        ]
        for (index, line) in lines.enumerated() {
            if index > 0 {
                let divider = NSBox()
                divider.boxType = .separator
                divider.fillColor = .lookupLine
                divider.translatesAutoresizingMaskIntoConstraints = false
                divider.heightAnchor.constraint(equalToConstant: 1).isActive = true
                views.append(divider)
            }

            let textStack = NSStackView()
            textStack.orientation = .vertical
            textStack.alignment = .leading
            textStack.spacing = 3
            textStack.setContentHuggingPriority(.defaultLow, for: .horizontal)
            textStack.addArrangedSubview(makeLabel(
                line.pinyin,
                font: chineseFont(size: 14, weight: .semibold),
                color: .lookupNavy
            ))
            textStack.addArrangedSubview(makeLabel(
                line.meaning,
                font: chineseFont(size: 12, weight: .regular),
                color: .lookupSecondary
            ))

            let row = NSStackView(views: [
                textStack,
                makeButton(title: buttonTitle(for: line), tag: index, fullWidth: false),
            ])
            row.orientation = .horizontal
            row.alignment = .centerY
            row.spacing = 8
            row.distribution = .fill
            views.append(row)
        }
        return views
    }

    private func makeLabel(_ text: String, font: NSFont, color: NSColor) -> NSTextField {
        let label = NSTextField(labelWithString: text)
        label.font = font
        label.textColor = color
        label.alignment = .left
        label.lineBreakMode = .byWordWrapping
        label.maximumNumberOfLines = 0
        label.preferredMaxLayoutWidth = 312
        label.setContentHuggingPriority(.defaultLow, for: .horizontal)
        label.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        return label
    }

    private func makeButton(title: String, tag: Int, fullWidth: Bool) -> NSButton {
        let button = PaddedButton(title: title, target: self, action: #selector(addPressed(_:)))
        button.tag = tag
        button.isBordered = false
        let font = NSFont.systemFont(ofSize: 12, weight: .semibold)
        button.font = font
        button.attributedTitle = NSAttributedString(
            string: title,
            attributes: [
                .foregroundColor: NSColor.white,
                .font: font,
            ]
        )
        button.wantsLayer = true
        button.layer?.backgroundColor = NSColor.lookupButton.cgColor
        button.layer?.cornerRadius = 7
        button.layer?.masksToBounds = true
        button.setAccessibilityLabel(title)
        button.heightAnchor.constraint(equalToConstant: fullWidth ? 28 : 26).isActive = true
        if fullWidth {
            button.widthAnchor.constraint(equalToConstant: 312).isActive = true
        } else {
            button.widthAnchor.constraint(greaterThanOrEqualToConstant: 84).isActive = true
        }
        return button
    }

    private func buttonTitle(for line: GlossLine) -> String {
        line.action == "open" ? "Details" : "Add word"
    }

    private func chineseFont(size: CGFloat, weight: NSFont.Weight) -> NSFont {
        NSFont.systemFont(ofSize: size, weight: weight)
    }

    private func show(at anchor: PanelAnchor) {
        lastAnchor = anchor
        panel.contentView?.layoutSubtreeIfNeeded()
        let fittingHeight = panel.contentView?.fittingSize.height ?? 100
        let maxHeight = max(240, (NSScreen.main?.visibleFrame.height ?? 800) - 80)
        let height = min(max(80, fittingHeight), maxHeight)
        panel.setContentSize(NSSize(width: panelWidth, height: height))
        position(using: anchor)
        panel.orderFrontRegardless()
    }

    private func position(using anchor: PanelAnchor) {
        let referencePoint = anchor.selectionRect?.origin ?? anchor.mouseLocation
        let screen = NSScreen.screens.first(where: {
            $0.frame.contains(referencePoint)
        }) ?? NSScreen.main ?? NSScreen.screens[0]
        let visibleFrame = screen.visibleFrame
        let size = panel.frame.size
        let gap: CGFloat = 8
        let selectionRect = anchor.selectionRect.flatMap { rect in
            rect.width > 0 && rect.height > 0 ? rect : nil
        }

        let proposedX = selectionRect.map {
            $0.midX - size.width / 2
        } ?? (anchor.mouseLocation.x + 14)
        let x = min(
            max(proposedX, visibleFrame.minX + 8),
            max(visibleFrame.minX + 8, visibleFrame.maxX - size.width - 8)
        )
        let minimumY = visibleFrame.minY + 8
        let maximumY = max(minimumY, visibleFrame.maxY - size.height - 8)
        let clampedY: (CGFloat) -> CGFloat = { value in
            min(max(value, minimumY), maximumY)
        }

        var y: CGFloat
        if let selectionRect {
            let belowY = clampedY(selectionRect.minY - gap - size.height)
            let belowFrame = CGRect(
                x: x,
                y: belowY,
                width: size.width,
                height: size.height
            )
            if belowFrame.intersects(selectionRect) {
                y = clampedY(selectionRect.maxY + gap)
            } else {
                y = belowY
            }
        } else {
            y = clampedY(anchor.mouseLocation.y - size.height - 14)
        }

        panel.setFrame(
            NSRect(x: x, y: y, width: size.width, height: size.height),
            display: true
        )
    }

    private func makeCardFields(_ fields: CardFields, editable: Bool) -> [NSView] {
        let rows: [(String, String, String, Bool)] = [
            ("simplified", "Simplified", fields.simplified, false),
            ("traditional", "Traditional", fields.traditional ?? "", false),
            ("pinyin", "Pinyin", fields.pinyin, false),
            ("meaning", "Meaning", fields.meaning, true),
            ("partOfSpeech", "Part of speech", fields.partOfSpeech ?? "", false),
            ("classifier", "Classifier", fields.classifier ?? "", false),
            ("example", "Example", fields.example ?? "", true),
            ("examplePinyin", "Example pinyin", fields.examplePinyin ?? "", false),
            ("exampleTranslation", "Translation", fields.exampleTranslation ?? "", true),
            ("tags", "Tags", (fields.tags ?? []).joined(separator: ", "), false),
        ]
        return rows.map { key, title, value, multiline in
            makeField(key: key, title: title, value: value, editable: editable, multiline: multiline)
        }
    }

    private func makeField(
        key: String,
        title: String,
        value: String,
        editable: Bool,
        multiline: Bool
    ) -> NSView {
        let label = makeLabel(
            title,
            font: chineseFont(size: 11, weight: .medium),
            color: .lookupSecondary
        )
        let input = NSTextField(string: value)
        input.font = chineseFont(size: 12, weight: .regular)
        input.textColor = .lookupInk
        input.isEditable = editable
        input.isSelectable = true
        input.isBezeled = editable
        input.isBordered = editable
        input.bezelStyle = .roundedBezel
        input.drawsBackground = editable
        input.backgroundColor = editable ? .white : NSColor(calibratedWhite: 0.97, alpha: 1)
        input.lineBreakMode = .byWordWrapping
        input.maximumNumberOfLines = multiline ? 3 : 1
        input.cell?.wraps = multiline
        input.cell?.isScrollable = !multiline
        input.translatesAutoresizingMaskIntoConstraints = false
        input.widthAnchor.constraint(equalToConstant: 312).isActive = true
        input.heightAnchor.constraint(equalToConstant: multiline ? 44 : 22).isActive = true
        fieldInputs[key] = input
        let stack = NSStackView(views: [label, input])
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 2
        return stack
    }

    private func makeChoiceButton(title: String, tag: Int) -> NSButton {
        let button = PaddedButton(title: title, target: self, action: #selector(chooseVariant(_:)))
        button.tag = tag
        button.isBordered = false
        let font = NSFont.systemFont(ofSize: 12, weight: .medium)
        button.font = font
        button.attributedTitle = NSAttributedString(
            string: title,
            attributes: [
                .foregroundColor: NSColor.lookupNavy,
                .font: font,
            ]
        )
        button.alignment = .left
        button.wantsLayer = true
        button.layer?.backgroundColor = NSColor(calibratedRed: 0xE9 / 255.0, green: 0xF1 / 255.0, blue: 0xFD / 255.0, alpha: 1).cgColor
        button.layer?.cornerRadius = 6
        button.setAccessibilityLabel(title)
        button.widthAnchor.constraint(equalToConstant: 312).isActive = true
        button.heightAnchor.constraint(greaterThanOrEqualToConstant: 26).isActive = true
        return button
    }

    private func makeSaveButton() -> NSButton {
        let button = PaddedButton(title: "Save", target: self, action: #selector(savePressed))
        button.isBordered = false
        let font = NSFont.systemFont(ofSize: 12, weight: .semibold)
        button.font = font
        button.attributedTitle = NSAttributedString(
            string: "Save",
            attributes: [
                .foregroundColor: NSColor.white,
                .font: font,
            ]
        )
        button.wantsLayer = true
        button.layer?.backgroundColor = NSColor.lookupButton.cgColor
        button.layer?.cornerRadius = 7
        button.setAccessibilityLabel("Save")
        button.widthAnchor.constraint(equalToConstant: 312).isActive = true
        button.heightAnchor.constraint(equalToConstant: 28).isActive = true
        return button
    }

    private func apply(variant: CardVariant) {
        setField("traditional", variant.traditional ?? "")
        setField("pinyin", variant.pinyin)
        setField("meaning", variant.meaning)
        setField("partOfSpeech", variant.partOfSpeech ?? "")
        setField("classifier", variant.classifier ?? "")
        setField("example", variant.example ?? "")
        setField("examplePinyin", variant.examplePinyin ?? "")
        setField("exampleTranslation", variant.exampleTranslation ?? "")
        generatedCard = true
    }

    private func setField(_ key: String, _ value: String) {
        fieldInputs[key]?.stringValue = value
    }

    private func fieldValue(_ key: String) -> String {
        fieldInputs[key]?.stringValue.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }

    private func currentFields() -> CardFields {
        let tags = fieldValue("tags")
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        return CardFields(
            simplified: fieldValue("simplified"),
            traditional: fieldValue("traditional").isEmpty ? nil : fieldValue("traditional"),
            pinyin: fieldValue("pinyin"),
            meaning: fieldValue("meaning"),
            partOfSpeech: fieldValue("partOfSpeech").isEmpty ? nil : fieldValue("partOfSpeech"),
            classifier: fieldValue("classifier").isEmpty ? nil : fieldValue("classifier"),
            example: fieldValue("example").isEmpty ? nil : fieldValue("example"),
            examplePinyin: fieldValue("examplePinyin").isEmpty ? nil : fieldValue("examplePinyin"),
            exampleTranslation: fieldValue("exampleTranslation").isEmpty ? nil : fieldValue("exampleTranslation"),
            tags: tags.isEmpty ? nil : tags,
            aiGenerated: generatedCard
        )
    }

    @objc private func chooseVariant(_ sender: NSButton) {
        guard cardVariants.indices.contains(sender.tag) else {
            return
        }
        apply(variant: cardVariants[sender.tag])
        showFormStatus("")
    }

    @objc private func savePressed() {
        onSave?(currentFields())
    }

    @objc private func closePressed() {
        onDismiss?()
    }

    @objc private func addPressed(_ sender: NSButton) {
        guard currentLines.indices.contains(sender.tag) else {
            return
        }
        onAdd?(currentLines[sender.tag])
    }
}
