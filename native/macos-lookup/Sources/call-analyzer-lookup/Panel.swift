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
}

struct PanelAnchor {
    let selectionRect: CGRect?
    let mouseLocation: NSPoint
}

@MainActor
final class PanelController: NSObject {
    var onDismiss: (() -> Void)?
    var onAdd: ((GlossLine) -> Void)?

    private let panel: LookupPanel
    private let bodyStack = NSStackView()
    private let closeButton: NSButton
    private var currentLines: [GlossLine] = []
    private var localMonitor: Any?
    private var globalMonitor: Any?

    override init() {
        panel = LookupPanel(
            contentRect: NSRect(x: 0, y: 0, width: 280, height: 120),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: true
        )
        closeButton = NSButton(title: "Close", target: nil, action: nil)
        super.init()
        configurePanel()
        installMonitors()
    }

    func showLoading(surface: String, anchor: PanelAnchor) {
        currentLines = []
        replaceBody(with: [
            makeLabel(
                surface,
                font: chineseFont(size: 18, weight: .medium),
                color: .lookupInk
            ),
            makeLabel(
                "Looking up…",
                font: chineseFont(size: 14, weight: .regular),
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
                    font: chineseFont(size: 18, weight: .medium),
                    color: .lookupInk
                ),
                makeLabel(
                    notice,
                    font: chineseFont(size: 14, weight: .regular),
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
                font: chineseFont(size: 14, weight: .regular),
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
                font: chineseFont(size: 14, weight: .regular),
                color: .lookupSecondary
            )
        ])
        show(at: anchor)
    }

    func showAdding(action: String, anchor: PanelAnchor) {
        currentLines = []
        let message = action == "open" ? "Opening…" : "Adding…"
        replaceBody(with: [
            makeLabel(
                message,
                font: chineseFont(size: 14, weight: .regular),
                color: .lookupSecondary
            )
        ])
        show(at: anchor)
    }

    func hide() {
        panel.orderOut(nil)
        currentLines = []
    }

    private func configurePanel() {
        panel.isFloatingPanel = true
        panel.level = .floating
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .transient]
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

    private func installMonitors() {
        localMonitor = NSEvent.addLocalMonitorForEvents(
            matching: [.keyDown, .leftMouseDown, .rightMouseDown, .otherMouseDown]
        ) { [weak self] event in
            guard let self else {
                return event
            }
            if event.type == .keyDown, event.keyCode == 53 {
                self.onDismiss?()
                return nil
            }
            if isMouseDown(event.type),
               self.panel.isVisible,
               !self.panel.frame.contains(NSEvent.mouseLocation) {
                self.onDismiss?()
                return nil
            }
            return event
        }

        globalMonitor = NSEvent.addGlobalMonitorForEvents(matching: [
            .keyDown,
            .leftMouseDown,
            .rightMouseDown,
            .otherMouseDown,
        ]) { [weak self] event in
            guard let self else {
                return
            }
            if event.type == .keyDown, event.keyCode == 53 {
                self.onDismiss?()
                return
            }
            if isMouseDown(event.type),
               self.panel.isVisible,
               !self.panel.frame.contains(NSEvent.mouseLocation) {
                self.onDismiss?()
            }
        }
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
            font: chineseFont(size: 18, weight: .medium),
            color: .lookupInk
        )
        let pinyin = makeLabel(
            line.pinyin,
            font: chineseFont(size: 20, weight: .semibold),
            color: .lookupNavy
        )
        let meaning = makeLabel(
            line.meaning,
            font: chineseFont(size: 14, weight: .regular),
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
                font: chineseFont(size: 18, weight: .medium),
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
                font: chineseFont(size: 16, weight: .semibold),
                color: .lookupNavy
            ))
            textStack.addArrangedSubview(makeLabel(
                line.meaning,
                font: chineseFont(size: 14, weight: .regular),
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
        label.preferredMaxLayoutWidth = 252
        label.setContentHuggingPriority(.defaultLow, for: .horizontal)
        label.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        return label
    }

    private func makeButton(title: String, tag: Int, fullWidth: Bool) -> NSButton {
        let button = PaddedButton(title: title, target: self, action: #selector(addPressed(_:)))
        button.tag = tag
        button.isBordered = false
        let font = NSFont.systemFont(ofSize: 14, weight: .semibold)
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
        button.heightAnchor.constraint(equalToConstant: fullWidth ? 34 : 30).isActive = true
        if fullWidth {
            button.widthAnchor.constraint(equalToConstant: 252).isActive = true
        } else {
            button.widthAnchor.constraint(greaterThanOrEqualToConstant: 84).isActive = true
        }
        return button
    }

    private func buttonTitle(for line: GlossLine) -> String {
        line.action == "open" ? "Open" : "Add word"
    }

    private func chineseFont(size: CGFloat, weight: NSFont.Weight) -> NSFont {
        NSFont.systemFont(ofSize: size, weight: weight)
    }

    private func show(at anchor: PanelAnchor) {
        panel.contentView?.layoutSubtreeIfNeeded()
        let fittingHeight = panel.contentView?.fittingSize.height ?? 100
        panel.setContentSize(NSSize(width: 280, height: max(80, fittingHeight)))
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

    @objc private func closePressed() {
        onDismiss?()
    }

    @objc private func addPressed(_ sender: NSButton) {
        guard currentLines.indices.contains(sender.tag) else {
            return
        }
        onAdd?(currentLines[sender.tag])
    }

    private func isMouseDown(_ type: NSEvent.EventType) -> Bool {
        switch type {
        case .leftMouseDown, .rightMouseDown, .otherMouseDown:
            return true
        default:
            return false
        }
    }
}
