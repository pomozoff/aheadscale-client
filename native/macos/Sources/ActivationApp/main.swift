import AppKit
import SystemExtensions
import AheadscaleActivation

final class SetupApp: NSObject, NSApplicationDelegate {
    private let controller = ActivationController()
    private let status = NSTextField(wrappingLabelWithString:
        "Прототип системного расширения. VPN-движок ещё не подключён.")
    private var window: NSWindow!

    func applicationDidFinishLaunching(_ notification: Notification) {
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 580, height: 220),
                          styleMask: [.titled, .closable, .miniaturizable],
                          backing: .buffered, defer: false)
        window.title = "Aheadscale — активация расширения"
        let title = NSTextField(labelWithString: "Aheadscale System Extension")
        title.font = .boldSystemFont(ofSize: 20)
        let activate = NSButton(title: "Активировать расширение", target: self, action: #selector(activate))
        let deactivate = NSButton(title: "Деактивировать", target: self, action: #selector(deactivate))
        let buttons = NSStackView(views: [activate, deactivate])
        buttons.orientation = .horizontal
        let stack = NSStackView(views: [title, status, buttons])
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 20
        stack.translatesAutoresizingMaskIntoConstraints = false
        let content = window.contentView!
        content.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 24),
            stack.trailingAnchor.constraint(equalTo: content.trailingAnchor, constant: -24),
            stack.topAnchor.constraint(equalTo: content.topAnchor, constant: 24)
        ])
        controller.onChange = { [weak self] state in self?.show(state) }
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc private func activate() { submit(activate: true) }
    @objc private func deactivate() { submit(activate: false) }

    private func submit(activate: Bool) {
        OSSystemExtensionManager.shared.submitRequest(controller.makeRequest(activate: activate))
    }

    private func show(_ state: ActivationState) {
        switch state {
        case .idle: break
        case .working: status.stringValue = "macOS обрабатывает запрос…"
        case .waitingForApproval: status.stringValue = "Разреши расширение Aheadscale в системных настройках macOS."
        case .active: status.stringValue = "Расширение активировано. VPN-движок ещё не подключён."
        case .inactive: status.stringValue = "Расширение деактивировано."
        case .rebootRequired: status.stringValue = "Для завершения операции macOS требуется перезагрузка."
        case .failed(let message): status.stringValue = "Операция не выполнена: " + message
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}

let app = NSApplication.shared
let delegate = SetupApp()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
