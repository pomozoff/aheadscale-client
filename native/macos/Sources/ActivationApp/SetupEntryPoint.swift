import AppKit
import SystemExtensions
import AheadscaleActivation

@MainActor
final class SetupApp: NSObject, NSApplicationDelegate {
    private let controller = ActivationController()
    private let vpn = PrototypeVPNController(nativeEngine: true)
    private let vpnStatus = NSTextField(wrappingLabelWithString: "Системный VPN выключен.")
    private let status = NSTextField(wrappingLabelWithString:
        "Проверка системного VPN с рабочим движком Aheadscale.")
    private var window: NSWindow!

    func applicationDidFinishLaunching(_ notification: Notification) {
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 580, height: 360),
                          styleMask: [.titled, .closable, .miniaturizable],
                          backing: .buffered, defer: false)
        window.title = "Aheadscale — активация расширения"
        let title = NSTextField(labelWithString: "Aheadscale System Extension")
        title.font = .boldSystemFont(ofSize: 20)
        let activate = NSButton(title: "Активировать расширение", target: self, action: #selector(activate))
        let deactivate = NSButton(title: "Деактивировать", target: self, action: #selector(deactivate))
        let buttons = NSStackView(views: [activate, deactivate])
        buttons.orientation = .horizontal
        let connect = NSButton(title: "Проверить запуск VPN", target: self, action: #selector(connectVPN))
        let disconnect = NSButton(title: "Отключить / Cancel", target: self, action: #selector(disconnectVPN))
        let remove = NSButton(title: "Удалить тестовый профиль", target: self, action: #selector(removeVPN))
        let vpnButtons = NSStackView(views: [connect, disconnect, remove])
        vpnButtons.orientation = .horizontal
        let stack = NSStackView(views: [title, status, buttons, vpnStatus, vpnButtons])
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
        vpn.onChange = { [weak self] state in
            switch state {
            case .disconnected: self?.vpnStatus.stringValue = "Тестовый VPN выключен."
            case .preparing: self?.vpnStatus.stringValue = "Подготовка тестового профиля; macOS может запросить разрешение."
            case .connected: self?.vpnStatus.stringValue = "Системный VPN подключён; движок подтвердил готовность."
            case .starting: self?.vpnStatus.stringValue = "Подключение системного VPN к движку Aheadscale…"
            case .stopping: self?.vpnStatus.stringValue = "Остановка тестового VPN…"
            case .failed(let message): self?.vpnStatus.stringValue = message
            }
        }
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc private func connectVPN() { vpn.connect() }
    @objc private func disconnectVPN() { vpn.disconnect() }
    @objc private func removeVPN() { vpn.removeProfile() }

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
        case .active: status.stringValue = "Расширение активировано. Можно подключить системный VPN."
        case .inactive: status.stringValue = "Расширение деактивировано."
        case .rebootRequired: status.stringValue = "Для завершения операции macOS требуется перезагрузка."
        case .failed(let message): status.stringValue = "Операция не выполнена: " + message
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}

@main
struct SetupEntryPoint {
    @MainActor static func main() {
        let app = NSApplication.shared
        let delegate = SetupApp()
        app.delegate = delegate
        app.setActivationPolicy(.regular)
        app.run()
        withExtendedLifetime(delegate) {}
    }
}
