import AppKit

private extension NSColor {
    static let lookupInk = NSColor(calibratedRed: 0x15 / 255.0, green: 0x20 / 255.0, blue: 0x33 / 255.0, alpha: 1)
    static let lookupSecondary = NSColor(calibratedRed: 0x4B / 255.0, green: 0x5A / 255.0, blue: 0x70 / 255.0, alpha: 1)
    static let lookupNavy = NSColor(calibratedRed: 0x20 / 255.0, green: 0x37 / 255.0, blue: 0x5E / 255.0, alpha: 1)
    static let lookupLine = NSColor(calibratedRed: 0xE2 / 255.0, green: 0xE7 / 255.0, blue: 0xEE / 255.0, alpha: 1)
    static let lookupLineStrong = NSColor(calibratedRed: 0xD2 / 255.0, green: 0xD9 / 255.0, blue: 0xE4 / 255.0, alpha: 1)
    static let lookupSurfaceMuted = NSColor(calibratedRed: 0xF5 / 255.0, green: 0xF7 / 255.0, blue: 0xFA / 255.0, alpha: 1)
    static let lookupButton = NSColor(calibratedRed: 0x39 / 255.0, green: 0x74 / 255.0, blue: 0xC8 / 255.0, alpha: 1)
    static let lookupBlueSoft = NSColor(calibratedRed: 0xE9 / 255.0, green: 0xF1 / 255.0, blue: 0xFD / 255.0, alpha: 1)
    static let lookupBlueTint = NSColor(calibratedRed: 0xF5 / 255.0, green: 0xF8 / 255.0, blue: 0xFE / 255.0, alpha: 1)
    static let lookupBlueBorder = NSColor(calibratedRed: 0xD2 / 255.0, green: 0xE1 / 255.0, blue: 0xFA / 255.0, alpha: 1)
    static let lookupGreen = NSColor(calibratedRed: 0x2E / 255.0, green: 0x92 / 255.0, blue: 0x70 / 255.0, alpha: 1)
    static let lookupGreenSoft = NSColor(calibratedRed: 0xE8 / 255.0, green: 0xF6 / 255.0, blue: 0xF0 / 255.0, alpha: 1)
    static let lookupGreenBorder = NSColor(calibratedRed: 0xC6 / 255.0, green: 0xE7 / 255.0, blue: 0xDA / 255.0, alpha: 1)
    static let lookupDanger = NSColor(calibratedRed: 0xBB / 255.0, green: 0x59 / 255.0, blue: 0x60 / 255.0, alpha: 1)
    static let lookupDangerSoft = NSColor(calibratedRed: 0xFF / 255.0, green: 0xF0 / 255.0, blue: 0xF1 / 255.0, alpha: 1)
    static let lookupDangerBorder = NSColor(calibratedRed: 0xF2 / 255.0, green: 0xCF / 255.0, blue: 0xD2 / 255.0, alpha: 1)
    static let lookupDangerInk = NSColor(calibratedRed: 0x6E / 255.0, green: 0x32 / 255.0, blue: 0x38 / 255.0, alpha: 1)
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
        return NSSize(width: size.width + 24, height: max(size.height, 24))
    }
}

private final class FlowView: NSView {
    var preferredWidth: CGFloat = 312
    override var isFlipped: Bool { true }

    func setItems(_ items: [NSView]) {
        subviews.forEach { $0.removeFromSuperview() }
        items.forEach { addSubview($0) }
        invalidateIntrinsicContentSize()
        needsLayout = true
    }

    override var intrinsicContentSize: NSSize {
        layoutItems(in: preferredWidth, place: false)
    }

    override func layout() {
        super.layout()
        let width = bounds.width > 1 ? bounds.width : preferredWidth
        _ = layoutItems(in: width, place: true)
    }

    private func layoutItems(in maxWidth: CGFloat, place: Bool) -> NSSize {
        var x: CGFloat = 0
        var y: CGFloat = 0
        var rowHeight: CGFloat = 0
        for item in subviews {
            let fit = item.fittingSize
            let width = min(maxWidth, max(fit.width, 44))
            let height = max(fit.height, 24)
            if x > 0 && x + width > maxWidth + 0.5 {
                x = 0
                y += rowHeight + 6
                rowHeight = 0
            }
            if place {
                item.frame = NSRect(x: x, y: y, width: width, height: height)
            }
            x += width + 6
            rowHeight = max(rowHeight, height)
        }
        return NSSize(width: maxWidth, height: y + rowHeight)
    }
}

struct PanelAnchor {
    let selectionRect: CGRect?
    let mouseLocation: NSPoint
}

@MainActor
final class PanelController: NSObject, NSTextFieldDelegate {
    private static let regeneratePresets: [(title: String, prompt: String)] = [
        ("Simpler example", "Use a simpler, everyday example sentence."),
        ("More casual", "Make the example sentence more casual and conversational."),
        ("Different meaning", "Use a different common meaning or sense of this word."),
    ]

    var onDismiss: (() -> Void)?
    var onAdd: ((GlossLine) -> Void)?
    var onSave: ((CardFields) -> Void)?
    var onRetryAdd: (() -> Void)?
    var onRetryLookup: (() -> Void)?
    var onBackToResults: (() -> Void)?
    var onRegenerate: ((CardFields, String) -> Void)?

    private enum ActionButtonStyle {
        case primary
        case secondary
    }

    private let panel: LookupPanel
    private let bodyStack = NSStackView()
    private let scrollView = NSScrollView()
    private let closeButton: PaddedButton
    private let panelWidth: CGFloat = 340
    private var scrollHeight: NSLayoutConstraint?
    private var bodyHeight: NSLayoutConstraint?
    private var cursorTimer: Timer?
    private var usingCustomCursor = false
    private var currentLines: [GlossLine] = []
    private var fieldInputs: [String: NSTextField] = [:]
    private var formStatusContainer: NSStackView?
    private var formErrorContainer: NSStackView?
    private var formActionButtons: [NSButton] = []
    private var defaultActionRow: NSView?
    private var regenerateDrawer: NSView?
    private var regeneratePromptField: NSTextField?
    private var regeneratePresetButtons: [NSButton] = []
    private var generatedCard = false
    private var showingCard = false
    private var cardVariants: [CardVariant] = []
    private var existingTags: [String] = []
    private var tagButtons: [NSButton] = []
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
        showingCard = false
        clearFormReferences()
        replaceBody(with: [
            makeLabel(
                surface,
                font: chineseFont(size: 14, weight: .medium),
                color: .lookupInk
            ),
            makeSpinnerRow(message: "Looking up…"),
        ])
        show(at: anchor)
    }

    func showResult(
        surface: String,
        lines: [GlossLine],
        notice: String,
        existingTags: [String] = [],
        anchor: PanelAnchor
    ) {
        self.existingTags = existingTags
        showingCard = false
        clearFormReferences()
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
        showingCard = false
        clearFormReferences()
        replaceBody(with: [
            makeLabel(
                message,
                font: chineseFont(size: 12, weight: .regular),
                color: .lookupSecondary
            )
        ])
        show(at: anchor)
    }

    func showError(title: String, detail: String?, anchor: PanelAnchor) {
        currentLines = []
        showingCard = false
        clearFormReferences()
        replaceBody(with: [
            makeErrorCallout(title: title, detail: detail),
        ])
        show(at: anchor)
    }

    func showLookupError(
        surface: String,
        title: String,
        detail: String?,
        canRetry: Bool,
        anchor: PanelAnchor
    ) {
        currentLines = []
        showingCard = false
        clearFormReferences()
        var views: [NSView] = []
        let trimmedSurface = surface.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmedSurface.isEmpty {
            views.append(makeLabel(
                trimmedSurface,
                font: chineseFont(size: 14, weight: .medium),
                color: .lookupInk
            ))
        }
        views.append(makeErrorCallout(title: title, detail: detail))
        if canRetry {
            views.append(makeActionButton(
                title: "Retry",
                symbolName: "arrow.clockwise",
                style: .primary,
                action: #selector(retryLookupPressed),
                width: 312
            ))
        }
        replaceBody(with: views)
        show(at: anchor)
    }

    func showAddError(
        line: GlossLine?,
        title: String,
        detail: String?,
        canGoBack: Bool,
        anchor: PanelAnchor
    ) {
        currentLines = []
        showingCard = false
        clearFormReferences()
        var views: [NSView] = []
        if let line {
            views.append(contentsOf: makeWordContextViews(for: line))
        }
        views.append(makeErrorCallout(title: title, detail: detail))

        if canGoBack {
            let backButton = makeActionButton(
                title: "Back",
                symbolName: "chevron.left",
                style: .secondary,
                action: #selector(backToResultsPressed),
                width: 88
            )
            let retryButton = makeActionButton(
                title: "Retry",
                symbolName: "arrow.clockwise",
                style: .primary,
                action: #selector(retryAddPressed),
                width: 216
            )
            let actionRow = NSStackView(views: [backButton, retryButton])
            actionRow.orientation = .horizontal
            actionRow.alignment = .centerY
            actionRow.spacing = 8
            actionRow.distribution = .fill
            views.append(actionRow)
        } else {
            views.append(makeActionButton(
                title: "Retry",
                symbolName: "arrow.clockwise",
                style: .primary,
                action: #selector(retryAddPressed),
                width: 312
            ))
        }

        replaceBody(with: views)
        show(at: anchor)
    }

    func showAdding(line: GlossLine, anchor: PanelAnchor) {
        currentLines = []
        showingCard = false
        clearFormReferences()
        var views = makeWordContextViews(for: line)
        let message = line.action == "open" ? "Opening…" : "Generating card…"
        views.append(makeSpinnerRow(message: message))
        replaceBody(with: views)
        show(at: anchor)
    }

    func hide() {
        stopCursorTracking()
        panel.orderOut(nil)
        currentLines = []
        showingCard = false
        clearFormReferences()
    }

    func showCard(_ draft: CardDraft, notice: String? = nil, anchor: PanelAnchor) {
        currentLines = []
        showingCard = true
        cardVariants = draft.variants ?? []
        generatedCard = draft.fields.aiGenerated == true
        clearFormReferences()
        var views: [NSView] = []
        if let notice, !notice.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            views.append(makeSuccessBanner(message: notice))
        }
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

        if !draft.readOnly {
            let statusContainer = NSStackView()
            statusContainer.orientation = .vertical
            statusContainer.alignment = .width
            statusContainer.isHidden = true
            formStatusContainer = statusContainer
            views.append(statusContainer)

            let errorContainer = NSStackView()
            errorContainer.orientation = .vertical
            errorContainer.alignment = .width
            errorContainer.isHidden = true
            formErrorContainer = errorContainer
            views.append(errorContainer)

            let (drawer, drawerButtons) = makeRegenerateDrawer()
            drawer.isHidden = true
            regenerateDrawer = drawer
            views.append(drawer)

            let regenerateButton = makeActionButton(
                title: "Regenerate",
                symbolName: "arrow.clockwise",
                style: .secondary,
                action: #selector(openRegenerateDrawerPressed),
                width: 118
            )
            let saveButton = makeActionButton(
                title: "Save",
                symbolName: nil,
                style: .primary,
                action: #selector(savePressed),
                width: 186
            )
            formActionButtons = [regenerateButton, saveButton] + drawerButtons

            let actionRow = NSStackView(views: [regenerateButton, saveButton])
            actionRow.orientation = .horizontal
            actionRow.alignment = .centerY
            actionRow.spacing = 8
            actionRow.distribution = .fill
            defaultActionRow = actionRow
            views.append(actionRow)
        }
        replaceBody(with: views)
        show(at: anchor)
    }

    func shouldKeepForm() -> Bool {
        showingCard
    }

    func setCardBusy(message: String) {
        setFormButtonsEnabled(false)
        clearStack(formErrorContainer)
        formErrorContainer?.isHidden = true
        clearStack(formStatusContainer)
        formStatusContainer?.addArrangedSubview(makeSpinnerRow(message: message))
        formStatusContainer?.isHidden = false
        if let anchor = lastAnchor {
            show(at: anchor)
            scrollFormFeedbackIntoView()
        }
    }

    func showFormError(title: String, detail: String?) {
        setFormButtonsEnabled(true)
        clearStack(formStatusContainer)
        formStatusContainer?.isHidden = true
        clearStack(formErrorContainer)
        formErrorContainer?.addArrangedSubview(makeErrorCallout(title: title, detail: detail))
        formErrorContainer?.isHidden = false
        if let anchor = lastAnchor {
            show(at: anchor)
            scrollFormFeedbackIntoView()
        }
    }

    func clearFormFeedback() {
        setFormButtonsEnabled(true)
        clearStack(formStatusContainer)
        formStatusContainer?.isHidden = true
        clearStack(formErrorContainer)
        formErrorContainer?.isHidden = true
        if let anchor = lastAnchor {
            show(at: anchor)
        }
    }

    func currentTags() -> [String] {
        fieldValue("tags")
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
    }

    func showSaved(anchor: PanelAnchor) {
        showingCard = false
        clearFormReferences()
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
        panel.acceptsMouseMovedEvents = true
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
        bodyStack.translatesAutoresizingMaskIntoConstraints = false
        scrollView.drawsBackground = false
        scrollView.hasVerticalScroller = true
        scrollView.autohidesScrollers = true
        scrollView.borderType = .noBorder
        scrollView.documentView = bodyStack
        scrollView.translatesAutoresizingMaskIntoConstraints = false
        let clip = scrollView.contentView
        NSLayoutConstraint.activate([
            bodyStack.leadingAnchor.constraint(equalTo: clip.leadingAnchor),
            bodyStack.trailingAnchor.constraint(equalTo: clip.trailingAnchor),
            bodyStack.topAnchor.constraint(equalTo: clip.topAnchor),
            bodyStack.widthAnchor.constraint(equalTo: scrollView.widthAnchor),
        ])
        mainStack.addArrangedSubview(scrollView)
        let height = scrollView.heightAnchor.constraint(equalToConstant: 80)
        height.isActive = true
        scrollHeight = height
        let contentHeight = bodyStack.heightAnchor.constraint(equalToConstant: 80)
        contentHeight.isActive = true
        bodyHeight = contentHeight
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

    private func makeLabel(
        _ text: String,
        font: NSFont,
        color: NSColor,
        maxWidth: CGFloat = 312
    ) -> NSTextField {
        let label = NSTextField(labelWithString: text)
        label.font = font
        label.textColor = color
        label.alignment = .left
        label.lineBreakMode = .byWordWrapping
        label.maximumNumberOfLines = 0
        label.preferredMaxLayoutWidth = maxWidth
        label.setContentHuggingPriority(.defaultLow, for: .horizontal)
        label.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        return label
    }

    private func makeWordContextViews(for line: GlossLine) -> [NSView] {
        var views: [NSView] = []
        let heading = line.surface.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? line.simplified
            : line.surface
        if !heading.isEmpty {
            views.append(makeLabel(
                heading,
                font: chineseFont(size: 14, weight: .medium),
                color: .lookupInk
            ))
        }
        let pinyin = line.pinyin.trimmingCharacters(in: .whitespacesAndNewlines)
        let rawMeaning = line.meaning.trimmingCharacters(in: .whitespacesAndNewlines)
        let meaning = rawMeaning == "No English gloss saved" ? "" : rawMeaning
        let subtitle = [pinyin, meaning].filter { !$0.isEmpty }.joined(separator: " · ")
        if !subtitle.isEmpty {
            views.append(makeLabel(
                subtitle,
                font: chineseFont(size: 12, weight: .regular),
                color: .lookupSecondary
            ))
        }
        return views
    }

    private func makeSpinnerRow(message: String) -> NSView {
        let spinner = NSProgressIndicator()
        spinner.style = .spinning
        spinner.controlSize = .small
        spinner.isIndeterminate = true
        spinner.translatesAutoresizingMaskIntoConstraints = false
        spinner.widthAnchor.constraint(equalToConstant: 14).isActive = true
        spinner.heightAnchor.constraint(equalToConstant: 14).isActive = true
        spinner.startAnimation(nil)

        let label = makeLabel(
            message,
            font: chineseFont(size: 12, weight: .regular),
            color: .lookupSecondary,
            maxWidth: 290
        )
        let row = NSStackView(views: [spinner, label])
        row.orientation = .horizontal
        row.alignment = .centerY
        row.spacing = 7
        return row
    }

    private func makeErrorCallout(title: String, detail: String?) -> NSView {
        let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanDetail = detail?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""

        let container = NSView()
        container.wantsLayer = true
        container.layer?.backgroundColor = NSColor.lookupDangerSoft.cgColor
        container.layer?.borderColor = NSColor.lookupDangerBorder.cgColor
        container.layer?.borderWidth = 1
        container.layer?.cornerRadius = 8
        container.translatesAutoresizingMaskIntoConstraints = false
        container.widthAnchor.constraint(equalToConstant: 312).isActive = true

        let contentStack = NSStackView()
        contentStack.orientation = .vertical
        contentStack.alignment = .leading
        contentStack.spacing = 4
        contentStack.translatesAutoresizingMaskIntoConstraints = false

        let headerRow = NSStackView()
        headerRow.orientation = .horizontal
        headerRow.alignment = .centerY
        headerRow.spacing = 6

        var titleWidth: CGFloat = 292
        let symbolConfig = NSImage.SymbolConfiguration(pointSize: 12, weight: .semibold)
        if let iconImage = NSImage(
            systemSymbolName: "exclamationmark.triangle.fill",
            accessibilityDescription: nil
        )?.withSymbolConfiguration(symbolConfig) {
            iconImage.isTemplate = true
            let iconView = NSImageView(image: iconImage)
            iconView.contentTintColor = .lookupDanger
            iconView.translatesAutoresizingMaskIntoConstraints = false
            iconView.widthAnchor.constraint(equalToConstant: 14).isActive = true
            iconView.heightAnchor.constraint(equalToConstant: 14).isActive = true
            headerRow.addArrangedSubview(iconView)
            titleWidth = 272
        }

        let titleLabel = makeLabel(
            cleanTitle.isEmpty ? "Something went wrong." : cleanTitle,
            font: chineseFont(size: 12, weight: .semibold),
            color: .lookupDanger,
            maxWidth: titleWidth
        )
        headerRow.addArrangedSubview(titleLabel)
        contentStack.addArrangedSubview(headerRow)

        if !cleanDetail.isEmpty && cleanDetail.caseInsensitiveCompare(cleanTitle) != .orderedSame {
            let detailLabel = makeLabel(
                cleanDetail,
                font: chineseFont(size: 11.5, weight: .regular),
                color: .lookupDangerInk,
                maxWidth: 292
            )
            detailLabel.isSelectable = true
            contentStack.addArrangedSubview(detailLabel)
        }

        container.addSubview(contentStack)
        NSLayoutConstraint.activate([
            contentStack.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 10),
            contentStack.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -10),
            contentStack.topAnchor.constraint(equalTo: container.topAnchor, constant: 9),
            contentStack.bottomAnchor.constraint(equalTo: container.bottomAnchor, constant: -9),
        ])
        return container
    }

    private func makeSuccessBanner(message: String) -> NSView {
        let container = NSView()
        container.wantsLayer = true
        container.layer?.backgroundColor = NSColor.lookupGreenSoft.cgColor
        container.layer?.borderColor = NSColor.lookupGreenBorder.cgColor
        container.layer?.borderWidth = 1
        container.layer?.cornerRadius = 8
        container.translatesAutoresizingMaskIntoConstraints = false
        container.widthAnchor.constraint(equalToConstant: 312).isActive = true

        let row = NSStackView()
        row.orientation = .horizontal
        row.alignment = .centerY
        row.spacing = 6
        row.translatesAutoresizingMaskIntoConstraints = false

        var labelWidth: CGFloat = 292
        let symbolConfig = NSImage.SymbolConfiguration(pointSize: 12, weight: .semibold)
        if let iconImage = NSImage(
            systemSymbolName: "checkmark.circle.fill",
            accessibilityDescription: nil
        )?.withSymbolConfiguration(symbolConfig) {
            iconImage.isTemplate = true
            let iconView = NSImageView(image: iconImage)
            iconView.contentTintColor = .lookupGreen
            iconView.translatesAutoresizingMaskIntoConstraints = false
            iconView.widthAnchor.constraint(equalToConstant: 14).isActive = true
            iconView.heightAnchor.constraint(equalToConstant: 14).isActive = true
            row.addArrangedSubview(iconView)
            labelWidth = 272
        }

        row.addArrangedSubview(makeLabel(
            message,
            font: chineseFont(size: 11.5, weight: .medium),
            color: .lookupGreen,
            maxWidth: labelWidth
        ))

        container.addSubview(row)
        NSLayoutConstraint.activate([
            row.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 10),
            row.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -10),
            row.topAnchor.constraint(equalTo: container.topAnchor, constant: 7),
            row.bottomAnchor.constraint(equalTo: container.bottomAnchor, constant: -7),
        ])
        return container
    }

    private func makeRegenerateDrawer() -> (NSView, [NSButton]) {
        let container = NSView()
        container.wantsLayer = true
        container.layer?.backgroundColor = NSColor.lookupBlueTint.cgColor
        container.layer?.borderColor = NSColor.lookupBlueBorder.cgColor
        container.layer?.borderWidth = 1
        container.layer?.cornerRadius = 8
        container.translatesAutoresizingMaskIntoConstraints = false
        container.widthAnchor.constraint(equalToConstant: 312).isActive = true

        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 7
        stack.translatesAutoresizingMaskIntoConstraints = false

        let headerRow = NSStackView()
        headerRow.orientation = .horizontal
        headerRow.alignment = .centerY
        headerRow.spacing = 5
        let symbolConfig = NSImage.SymbolConfiguration(pointSize: 11, weight: .semibold)
        if let icon = NSImage(
            systemSymbolName: "sparkles",
            accessibilityDescription: nil
        )?.withSymbolConfiguration(symbolConfig) {
            icon.isTemplate = true
            let iconView = NSImageView(image: icon)
            iconView.contentTintColor = .lookupButton
            iconView.translatesAutoresizingMaskIntoConstraints = false
            iconView.widthAnchor.constraint(equalToConstant: 13).isActive = true
            iconView.heightAnchor.constraint(equalToConstant: 13).isActive = true
            headerRow.addArrangedSubview(iconView)
        }
        headerRow.addArrangedSubview(makeLabel(
            "Regenerate card",
            font: chineseFont(size: 12, weight: .semibold),
            color: .lookupNavy,
            maxWidth: 160
        ))
        let optionalHint = makeLabel(
            "Optional instructions",
            font: chineseFont(size: 11, weight: .regular),
            color: .lookupSecondary,
            maxWidth: 120
        )
        optionalHint.alignment = .right
        let spacer = NSView()
        spacer.setContentHuggingPriority(.fittingSizeCompression, for: .horizontal)
        headerRow.addArrangedSubview(spacer)
        headerRow.addArrangedSubview(optionalHint)
        headerRow.widthAnchor.constraint(equalToConstant: 292).isActive = true
        stack.addArrangedSubview(headerRow)

        let inputBox = NSView()
        inputBox.wantsLayer = true
        inputBox.layer?.cornerRadius = 6
        inputBox.layer?.borderWidth = 1
        inputBox.layer?.borderColor = NSColor.lookupLineStrong.cgColor
        inputBox.layer?.backgroundColor = NSColor.white.cgColor
        inputBox.translatesAutoresizingMaskIntoConstraints = false
        inputBox.widthAnchor.constraint(equalToConstant: 292).isActive = true
        inputBox.heightAnchor.constraint(equalToConstant: 46).isActive = true

        let promptField = NSTextField(string: "")
        promptField.font = chineseFont(size: 12, weight: .regular)
        promptField.textColor = .lookupInk
        promptField.placeholderString = "e.g. casual spoken example, simpler, business sense…"
        promptField.isEditable = true
        promptField.isSelectable = true
        promptField.isBezeled = false
        promptField.isBordered = false
        promptField.drawsBackground = false
        promptField.focusRingType = .none
        promptField.lineBreakMode = .byWordWrapping
        promptField.maximumNumberOfLines = 2
        promptField.cell?.wraps = true
        promptField.cell?.isScrollable = false
        promptField.delegate = self
        promptField.setAccessibilityLabel("Regeneration instructions")
        promptField.translatesAutoresizingMaskIntoConstraints = false
        inputBox.addSubview(promptField)
        NSLayoutConstraint.activate([
            promptField.leadingAnchor.constraint(equalTo: inputBox.leadingAnchor, constant: 8),
            promptField.trailingAnchor.constraint(equalTo: inputBox.trailingAnchor, constant: -8),
            promptField.topAnchor.constraint(equalTo: inputBox.topAnchor, constant: 6),
            promptField.bottomAnchor.constraint(equalTo: inputBox.bottomAnchor, constant: -6),
        ])
        regeneratePromptField = promptField
        stack.addArrangedSubview(inputBox)

        let flow = FlowView()
        flow.preferredWidth = 292
        flow.translatesAutoresizingMaskIntoConstraints = false
        regeneratePresetButtons = Self.regeneratePresets.enumerated().map { index, preset in
            let button = PaddedButton(
                title: preset.title,
                target: self,
                action: #selector(selectRegeneratePreset(_:))
            )
            button.tag = index
            button.isBordered = false
            button.font = chineseFont(size: 11, weight: .medium)
            button.setAccessibilityLabel(preset.title)
            button.translatesAutoresizingMaskIntoConstraints = true
            stylePresetButton(button)
            return button
        }
        flow.setItems(regeneratePresetButtons)
        let fitted = flow.intrinsicContentSize
        flow.widthAnchor.constraint(equalToConstant: 292).isActive = true
        flow.heightAnchor.constraint(equalToConstant: max(fitted.height, 24)).isActive = true
        stack.addArrangedSubview(flow)

        let cancelButton = makeActionButton(
            title: "Cancel",
            symbolName: nil,
            style: .secondary,
            action: #selector(closeRegenerateDrawerPressed),
            width: 86
        )
        let submitButton = makeActionButton(
            title: "Regenerate",
            symbolName: "arrow.clockwise",
            style: .primary,
            action: #selector(submitRegeneratePressed),
            width: 198
        )
        let drawerActions = NSStackView(views: [cancelButton, submitButton])
        drawerActions.orientation = .horizontal
        drawerActions.alignment = .centerY
        drawerActions.spacing = 8
        drawerActions.distribution = .fill
        stack.addArrangedSubview(drawerActions)

        container.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 10),
            stack.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -10),
            stack.topAnchor.constraint(equalTo: container.topAnchor, constant: 9),
            stack.bottomAnchor.constraint(equalTo: container.bottomAnchor, constant: -9),
        ])
        return (container, [cancelButton, submitButton] + regeneratePresetButtons)
    }

    private func stylePresetButton(_ button: NSButton) {
        guard Self.regeneratePresets.indices.contains(button.tag) else {
            return
        }
        let preset = Self.regeneratePresets[button.tag]
        let currentPrompt = regeneratePromptField?.stringValue.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let selected = currentPrompt.caseInsensitiveCompare(preset.prompt) == .orderedSame
        button.wantsLayer = true
        button.layer?.cornerRadius = 8
        button.layer?.borderWidth = 1
        let color: NSColor = selected ? .lookupNavy : .lookupSecondary
        let font = chineseFont(size: 11, weight: .medium)
        button.attributedTitle = NSAttributedString(
            string: preset.title,
            attributes: [.foregroundColor: color, .font: font]
        )
        if selected {
            button.layer?.backgroundColor = NSColor.lookupBlueSoft.cgColor
            button.layer?.borderColor = NSColor.lookupButton.cgColor
        } else {
            button.layer?.backgroundColor = NSColor.white.cgColor
            button.layer?.borderColor = NSColor.lookupLine.cgColor
        }
    }

    private func makeActionButton(
        title: String,
        symbolName: String?,
        style: ActionButtonStyle,
        action: Selector,
        width: CGFloat
    ) -> NSButton {
        let button = PaddedButton(title: title, target: self, action: action)
        button.isBordered = false
        let font = NSFont.systemFont(ofSize: 12, weight: .semibold)
        button.font = font

        let foregroundColor: NSColor
        switch style {
        case .primary:
            foregroundColor = .white
        case .secondary:
            foregroundColor = .lookupNavy
        }

        button.attributedTitle = NSAttributedString(
            string: title,
            attributes: [
                .foregroundColor: foregroundColor,
                .font: font,
            ]
        )

        if let symbolName {
            let symbolConfig = NSImage.SymbolConfiguration(pointSize: 11, weight: .semibold)
            if let image = NSImage(
                systemSymbolName: symbolName,
                accessibilityDescription: nil
            )?.withSymbolConfiguration(symbolConfig) {
                image.isTemplate = true
                button.image = image
                button.imagePosition = .imageLeading
                button.imageHugsTitle = true
                button.contentTintColor = foregroundColor
            }
        }

        button.wantsLayer = true
        button.layer?.cornerRadius = 7
        button.layer?.masksToBounds = true
        switch style {
        case .primary:
            button.layer?.backgroundColor = NSColor.lookupButton.cgColor
            button.layer?.borderWidth = 0
        case .secondary:
            button.layer?.backgroundColor = NSColor.lookupSurfaceMuted.cgColor
            button.layer?.borderWidth = 1
            button.layer?.borderColor = NSColor.lookupLineStrong.cgColor
        }

        button.setAccessibilityLabel(title)
        button.heightAnchor.constraint(equalToConstant: 28).isActive = true
        button.widthAnchor.constraint(equalToConstant: width).isActive = true
        return button
    }

    private func clearFormReferences() {
        fieldInputs = [:]
        formStatusContainer = nil
        formErrorContainer = nil
        formActionButtons = []
        defaultActionRow = nil
        regenerateDrawer = nil
        regeneratePromptField = nil
        regeneratePresetButtons = []
    }

    private func clearStack(_ stack: NSStackView?) {
        guard let stack else {
            return
        }
        for view in stack.arrangedSubviews {
            stack.removeArrangedSubview(view)
            view.removeFromSuperview()
        }
    }

    private func setFormButtonsEnabled(_ enabled: Bool) {
        for button in formActionButtons {
            button.isEnabled = enabled
            button.alphaValue = enabled ? 1.0 : 0.55
        }
        regeneratePromptField?.isEditable = enabled
        regeneratePromptField?.isEnabled = enabled
    }

    private func scrollFormFeedbackIntoView() {
        panel.contentView?.layoutSubtreeIfNeeded()
        if let drawer = regenerateDrawer, !drawer.isHidden {
            drawer.scrollToVisible(drawer.bounds)
        } else if let actionRow = defaultActionRow, !actionRow.isHidden {
            actionRow.scrollToVisible(actionRow.bounds)
        } else if let errorContainer = formErrorContainer, !errorContainer.isHidden {
            errorContainer.scrollToVisible(errorContainer.bounds)
        }
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
        bodyHeight?.isActive = false
        panel.contentView?.layoutSubtreeIfNeeded()
        let contentHeight = max(bodyStack.fittingSize.height, 24)
        bodyHeight?.constant = contentHeight
        bodyHeight?.isActive = true
        let chrome: CGFloat = 62
        let screenHeight = NSScreen.main?.visibleFrame.height ?? 800
        let maxScroll = max(120, screenHeight - 120 - chrome)
        scrollHeight?.constant = min(contentHeight, maxScroll)
        let height = (scrollHeight?.constant ?? contentHeight) + chrome
        panel.setContentSize(NSSize(width: panelWidth, height: height))
        position(using: anchor)
        panel.orderFrontRegardless()
        startCursorTracking()
    }

    private func startCursorTracking() {
        cursorTimer?.invalidate()
        let timer = Timer(timeInterval: 1.0 / 30.0, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated {
                self?.updatePointerCursor()
            }
        }
        RunLoop.main.add(timer, forMode: .common)
        cursorTimer = timer
    }

    private func stopCursorTracking() {
        cursorTimer?.invalidate()
        cursorTimer = nil
    }

    private func updatePointerCursor() {
        guard panel.isVisible else {
            return
        }
        let mouse = NSEvent.mouseLocation
        guard panel.frame.contains(mouse), let content = panel.contentView else {
            if usingCustomCursor {
                NSCursor.arrow.set()
                usingCustomCursor = false
            }
            return
        }
        let windowPoint = panel.convertPoint(fromScreen: mouse)
        let localPoint = content.convert(windowPoint, from: nil)
        var view = content.hitTest(localPoint)
        var hoveringButton = false
        var hoveringField = false
        while let current = view {
            if let button = current as? NSButton, button.isEnabled {
                hoveringButton = true
                break
            }
            if current is NSTextField, (current as? NSTextField)?.isEditable == true {
                hoveringField = true
            }
            view = current.superview
        }
        if hoveringButton {
            NSCursor.pointingHand.set()
            usingCustomCursor = true
        } else if hoveringField {
            NSCursor.iBeam.set()
            usingCustomCursor = true
        } else if usingCustomCursor {
            NSCursor.arrow.set()
            usingCustomCursor = false
        }
    }

    private func fieldBoxHeight(_ text: String, font: NSFont, multiline: Bool) -> CGFloat {
        guard multiline else {
            return 30
        }
        let measured = (text as NSString).boundingRect(
            with: NSSize(width: 296, height: 800),
            options: [.usesLineFragmentOrigin, .usesFontLeading],
            attributes: [.font: font]
        )
        return min(96, max(36, ceil(measured.height) + 14))
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
            ("pinyin", "Pinyin", fields.pinyin, true),
            ("meaning", "Meaning", fields.meaning, true),
            ("partOfSpeech", "Part of speech", fields.partOfSpeech ?? "", false),
            ("classifier", "Classifier", fields.classifier ?? "", false),
            ("example", "Example", fields.example ?? "", true),
            ("examplePinyin", "Example pinyin", fields.examplePinyin ?? "", true),
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
        let box = NSView()
        box.wantsLayer = true
        box.layer?.cornerRadius = 6
        box.layer?.borderWidth = 1
        box.layer?.borderColor = NSColor.lookupLine.cgColor
        box.layer?.backgroundColor = NSColor.white.cgColor
        box.translatesAutoresizingMaskIntoConstraints = false
        box.widthAnchor.constraint(equalToConstant: 312).isActive = true
        let font = chineseFont(size: 12, weight: .regular)
        let boxHeight = fieldBoxHeight(value, font: font, multiline: multiline)
        box.heightAnchor.constraint(equalToConstant: boxHeight).isActive = true

        let input = NSTextField(string: value)
        input.font = font
        input.textColor = .lookupInk
        input.isEditable = editable
        input.isSelectable = true
        input.isBezeled = false
        input.isBordered = false
        input.drawsBackground = false
        input.focusRingType = .none
        input.lineBreakMode = .byWordWrapping
        input.maximumNumberOfLines = multiline ? 3 : 1
        input.cell?.wraps = multiline
        input.cell?.isScrollable = !multiline
        input.translatesAutoresizingMaskIntoConstraints = false
        box.addSubview(input)
        var fieldConstraints = [
            input.leadingAnchor.constraint(equalTo: box.leadingAnchor, constant: 8),
            input.trailingAnchor.constraint(equalTo: box.trailingAnchor, constant: -8),
        ]
        if multiline {
            fieldConstraints.append(input.topAnchor.constraint(equalTo: box.topAnchor, constant: 6))
            fieldConstraints.append(input.bottomAnchor.constraint(equalTo: box.bottomAnchor, constant: -6))
        } else {
            fieldConstraints.append(input.centerYAnchor.constraint(equalTo: box.centerYAnchor))
        }
        NSLayoutConstraint.activate(fieldConstraints)
        fieldInputs[key] = input

        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 2
        stack.addArrangedSubview(label)
        if key == "tags" {
            stack.addArrangedSubview(makeLabel(
                "Separate tags with commas",
                font: chineseFont(size: 11, weight: .regular),
                color: .lookupSecondary
            ))
        }
        stack.addArrangedSubview(box)
        if key == "tags" && editable && !existingTags.isEmpty {
            stack.addArrangedSubview(makeExistingTagPicker())
        }
        return stack
    }

    private func makeExistingTagPicker() -> NSView {
        let caption = makeLabel(
            "Existing tags",
            font: chineseFont(size: 11, weight: .medium),
            color: .lookupSecondary
        )
        let flow = FlowView()
        flow.translatesAutoresizingMaskIntoConstraints = false
        tagButtons = existingTags.map { tag in
            let button = PaddedButton(title: tag, target: self, action: #selector(toggleExistingTag(_:)))
            button.isBordered = false
            button.font = chineseFont(size: 11, weight: .medium)
            button.lineBreakMode = .byTruncatingTail
            button.setAccessibilityLabel("Tag \(tag)")
            button.translatesAutoresizingMaskIntoConstraints = true
            styleTagButton(button)
            return button
        }
        flow.setItems(tagButtons)
        let fitted = flow.intrinsicContentSize
        flow.widthAnchor.constraint(equalToConstant: 312).isActive = true
        flow.heightAnchor.constraint(equalToConstant: max(fitted.height, 24)).isActive = true
        let stack = NSStackView(views: [caption, flow])
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 4
        return stack
    }

    private func styleTagButton(_ button: NSButton) {
        let selected = selectedTagNames().contains(button.title.lowercased())
        button.wantsLayer = true
        button.layer?.cornerRadius = 8
        button.layer?.borderWidth = 1
        let color: NSColor = selected ? .lookupNavy : .lookupInk
        let font = chineseFont(size: 11, weight: .medium)
        button.attributedTitle = NSAttributedString(
            string: button.title,
            attributes: [.foregroundColor: color, .font: font]
        )
        if selected {
            button.layer?.backgroundColor = NSColor(calibratedRed: 0xE9 / 255.0, green: 0xF1 / 255.0, blue: 0xFD / 255.0, alpha: 1).cgColor
            button.layer?.borderColor = NSColor.lookupButton.cgColor
        } else {
            button.layer?.backgroundColor = NSColor.white.cgColor
            button.layer?.borderColor = NSColor.lookupLine.cgColor
        }
    }

    private func selectedTagNames() -> Set<String> {
        Set(fieldValue("tags").split(separator: ",").map {
            $0.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        }.filter { !$0.isEmpty })
    }

    @objc private func toggleExistingTag(_ sender: NSButton) {
        let tag = sender.title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !tag.isEmpty else { return }
        var names = fieldValue("tags").split(separator: ",").map {
            $0.trimmingCharacters(in: .whitespacesAndNewlines)
        }.filter { !$0.isEmpty }
        if let index = names.firstIndex(where: { $0.caseInsensitiveCompare(tag) == .orderedSame }) {
            names.remove(at: index)
        } else {
            names.append(tag)
        }
        setField("tags", names.joined(separator: ", "))
        tagButtons.forEach(styleTagButton)
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
        let tags = currentTags()
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
        clearFormFeedback()
    }

    @objc private func savePressed() {
        onSave?(currentFields())
    }

    @objc private func openRegenerateDrawerPressed() {
        regenerateDrawer?.isHidden = false
        defaultActionRow?.isHidden = true
        clearStack(formErrorContainer)
        formErrorContainer?.isHidden = true
        if let anchor = lastAnchor {
            show(at: anchor)
            scrollFormFeedbackIntoView()
        }
        if let promptField = regeneratePromptField {
            panel.makeKey()
            panel.makeFirstResponder(promptField)
        }
    }

    @objc private func closeRegenerateDrawerPressed() {
        regenerateDrawer?.isHidden = true
        defaultActionRow?.isHidden = false
        clearStack(formErrorContainer)
        formErrorContainer?.isHidden = true
        if let anchor = lastAnchor {
            show(at: anchor)
            scrollFormFeedbackIntoView()
        }
    }

    @objc private func selectRegeneratePreset(_ sender: NSButton) {
        guard Self.regeneratePresets.indices.contains(sender.tag),
              let promptField = regeneratePromptField
        else {
            return
        }
        let preset = Self.regeneratePresets[sender.tag]
        let current = promptField.stringValue.trimmingCharacters(in: .whitespacesAndNewlines)
        if current.caseInsensitiveCompare(preset.prompt) == .orderedSame {
            promptField.stringValue = ""
        } else {
            promptField.stringValue = preset.prompt
        }
        regeneratePresetButtons.forEach(stylePresetButton)
        panel.makeFirstResponder(promptField)
    }

    @objc private func submitRegeneratePressed() {
        let instruction = regeneratePromptField?.stringValue.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        onRegenerate?(currentFields(), instruction)
    }

    func controlTextDidChange(_ obj: Notification) {
        guard let field = obj.object as? NSTextField, field === regeneratePromptField else {
            return
        }
        regeneratePresetButtons.forEach(stylePresetButton)
    }

    func control(
        _ control: NSControl,
        textView: NSTextView,
        doCommandBy commandSelector: Selector
    ) -> Bool {
        guard control === regeneratePromptField else {
            return false
        }
        if commandSelector == #selector(NSResponder.insertNewline(_:)) {
            submitRegeneratePressed()
            return true
        }
        if commandSelector == #selector(NSResponder.cancelOperation(_:)) {
            closeRegenerateDrawerPressed()
            return true
        }
        return false
    }

    @objc private func retryAddPressed() {
        onRetryAdd?()
    }

    @objc private func retryLookupPressed() {
        onRetryLookup?()
    }

    @objc private func backToResultsPressed() {
        onBackToResults?()
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
