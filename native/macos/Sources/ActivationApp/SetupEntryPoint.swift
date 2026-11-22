import AppKit
import Darwin
import Dispatch
import SystemExtensions
import AheadscaleActivation

@MainActor
final class SetupApp: NSObject, NSApplicationDelegate {
    private let controller = ActivationController()
    private let vpn: PrototypeVPNController?
    override init() {
        if let lease = try? VPNControlLease.userLease() {
            vpn = PrototypeVPNController(nativeEngine: true, lease: lease)
        } else { vpn = nil }
        super.init()
    }
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
        vpn?.onChange = { [weak self] state in
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

    @objc private func connectVPN() {
        guard let vpn else { vpnStatus.stringValue = "Не удалось согласовать системное управление VPN."; return }
        vpn.connect()
    }
    @objc private func disconnectVPN() { vpn?.disconnect() }
    @objc private func removeVPN() { vpn?.removeProfile() }

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

// The installed principal owns the activation request. The nested GUI is
// launched only after macOS has finished enabling the provider.
@MainActor
final class InstalledApp: NSObject, NSApplicationDelegate {
    private let activation = ActivationController()
    private let uninstall: Bool
    private var window: NSWindow?
    private let label = NSTextField(wrappingLabelWithString: "Подготовка системного VPN Aheadscale…")

    init(uninstall: Bool) { self.uninstall = uninstall; super.init() }

    func applicationDidFinishLaunching(_ notification: Notification) {
        activation.onChange = { [weak self] state in
            Task { @MainActor in self?.changed(state) }
        }
        if uninstall {
            Task { @MainActor in
                if await Self.removeVPN() { submit(activate: false) }
                else { changed(.failed("Не удалось удалить системный профиль Aheadscale.")) }
            }
        } else {
            submit(activate: true)
        }
    }

    private func submit(activate: Bool) {
        OSSystemExtensionManager.shared.submitRequest(activation.makeRequest(activate: activate))
    }

    private func changed(_ state: ActivationState) {
        switch state {
        case .active where !uninstall:
            let gui = Bundle.main.bundleURL.appendingPathComponent("Contents/Library/LoginItems/Aheadscale UI.app")
            let configuration = NSWorkspace.OpenConfiguration()
            configuration.createsNewApplicationInstance = true
            NSWorkspace.shared.openApplication(at: gui, configuration: configuration) { _, error in
                Task { @MainActor in
                    if let error { self.changed(.failed(error.localizedDescription)) }
                    else { exit(0) }
                }
            }
        case .inactive where uninstall: exit(0)
        case .waitingForApproval:
            show("Подтвердите действие для Aheadscale Tunnel в системных настройках macOS.")
        case .rebootRequired:
            fail("Перезагрузите Mac и повторите действие.")
        case .failed(let message):
            fail(message)
        default: break
        }
    }

    private func show(_ message: String) {
        label.stringValue = message
        if window == nil {
            let panel = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 490, height: 145),
                                 styleMask: [.titled, .closable], backing: .buffered, defer: false)
            panel.title = "Aheadscale"
            label.frame = NSRect(x: 24, y: 28, width: 440, height: 78)
            panel.contentView?.addSubview(label)
            panel.isReleasedWhenClosed = false
            panel.center()
            window = panel
        }
        window?.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    private func fail(_ message: String) {
        let dialog = NSAlert()
        dialog.messageText = "Aheadscale"
        dialog.informativeText = message
        NSApp.activate(ignoringOtherApps: true)
        dialog.runModal()
        exit(1)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        exit(1)
    }

    private static func removeVPN() async -> Bool {
        guard let lease = try? VPNControlLease.userLease() else { return false }
        let vpn = PrototypeVPNController(nativeEngine: true, lease: lease)
        do {
            if try await vpn.snapshot().profileExists == false { return true }
            vpn.disconnect()
            guard await waitForDisconnect(vpn) else { return false }
            vpn.removeProfile()
            guard await waitForDisconnect(vpn) else { return false }
            return try await vpn.snapshot().profileExists == false
        } catch { return false }
    }

    private static func waitForDisconnect(_ vpn: PrototypeVPNController) async -> Bool {
        let end = ProcessInfo.processInfo.systemUptime + 30
        while ProcessInfo.processInfo.systemUptime < end {
            guard vpn.requestIsCurrent else { return false }
            switch vpn.state {
            case .disconnected: return true
            case .failed: return false
            default: break
            }
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        return false
    }
}

@main
struct SetupEntryPoint {
    @MainActor static func main() {
        let arguments = Array(CommandLine.arguments.dropFirst())
        let installed = Bundle.main.object(forInfoDictionaryKey: "AheadscaleInstalled") as? Bool == true
        let uninstall = installed && arguments == ["--remove-system-vpn"]
        if !arguments.isEmpty && !uninstall {
            // Execute inside the principal bundle's provisioning context. Do not
            // create NSApplication or a window for machine-readable commands.
            Task { @MainActor in
                await VPNControlRunner.run(arguments)
                exit(0)
            }
            dispatchMain()
        }
        let app = NSApplication.shared
        if installed {
            guard geteuid() != 0, Bundle.main.bundleURL.path == "/Applications/Aheadscale.app" else { exit(1) }
        }
        let delegate: NSApplicationDelegate = installed ? InstalledApp(uninstall: uninstall) : SetupApp()
        app.delegate = delegate
        app.setActivationPolicy(.regular)
        app.run()
        withExtendedLifetime(delegate) {}
    }
}
