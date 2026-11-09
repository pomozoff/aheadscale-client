import Foundation
import SystemExtensions

public enum ActivationState: Equatable {
    case idle, working, waitingForApproval, active, inactive, rebootRequired
    case failed(String)
}

public final class ActivationController: NSObject, OSSystemExtensionRequestDelegate {
    public static let identifier = "ru.aheadscale.client.tunnel"
    public private(set) var state: ActivationState = .idle {
        didSet { onChange?(state) }
    }
    public var onChange: ((ActivationState) -> Void)?
    private var pending: OSSystemExtensionRequest?
    private var activating = false

    public func makeRequest(activate: Bool) -> OSSystemExtensionRequest {
        let request = activate
            ? OSSystemExtensionRequest.activationRequest(forExtensionWithIdentifier: Self.identifier, queue: .main)
            : OSSystemExtensionRequest.deactivationRequest(forExtensionWithIdentifier: Self.identifier, queue: .main)
        request.delegate = self
        activating = activate
        pending = request
        state = .working
        return request
    }

    public func requestNeedsUserApproval(_ request: OSSystemExtensionRequest) {
        guard request === pending else { return }
        state = .waitingForApproval
    }

    public func request(_ request: OSSystemExtensionRequest, didFailWithError error: Error) {
        guard request === pending else { return }
        pending = nil
        state = .failed(error.localizedDescription)
    }

    public func request(
        _ request: OSSystemExtensionRequest,
        didFinishWithResult result: OSSystemExtensionRequest.Result
    ) {
        guard request === pending else { return }
        pending = nil
        switch result {
        case .completed: state = activating ? .active : .inactive
        case .willCompleteAfterReboot: state = .rebootRequired
        @unknown default: state = .failed("Неизвестный результат активации macOS.")
        }
    }

    public func request(
        _ request: OSSystemExtensionRequest,
        actionForReplacingExtension existing: OSSystemExtensionProperties,
        withExtension replacement: OSSystemExtensionProperties
    ) -> OSSystemExtensionRequest.ReplacementAction {
        guard request === pending,
              existing.bundleIdentifier == Self.identifier,
              replacement.bundleIdentifier == Self.identifier else { return .cancel }
        return .replace
    }
}
