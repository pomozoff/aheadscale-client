import XCTest
import SystemExtensions
@testable import AheadscaleActivation

final class ActivationControllerTests: XCTestCase {
    func testApprovalAndFailureDoNotReportActive() {
        let controller = ActivationController()
        let request = controller.makeRequest(activate: true)
        controller.requestNeedsUserApproval(request)
        XCTAssertEqual(controller.state, .waitingForApproval)
        controller.request(request, didFailWithError: NSError(domain: "test", code: 3))
        guard case .failed = controller.state else {
            return XCTFail("An OS refusal must be visible, never active")
        }
    }

    func testRebootRequiredIsNotActive() {
        let controller = ActivationController()
        let request = controller.makeRequest(activate: true)
        controller.request(request, didFinishWithResult: .willCompleteAfterReboot)
        XCTAssertEqual(controller.state, .rebootRequired)
    }

    func testDeactivationAndLateCallbackCannotReportActive() {
        let controller = ActivationController()
        let old = controller.makeRequest(activate: true)
        let current = controller.makeRequest(activate: false)
        controller.request(old, didFinishWithResult: .completed)
        XCTAssertEqual(controller.state, .working)
        controller.request(current, didFinishWithResult: .completed)
        XCTAssertEqual(controller.state, .inactive)
        controller.request(old, didFinishWithResult: .completed)
        XCTAssertEqual(controller.state, .inactive)
    }

    func testSuccessfulActivationReportsExtensionActive() {
        let controller = ActivationController()
        let request = controller.makeRequest(activate: true)
        controller.request(request, didFinishWithResult: .completed)
        XCTAssertEqual(controller.state, .active)
    }
}
