import SwiftUI
import UIKit

extension UITextContentType {
    /// The AutoFill type of a login field (username, password, new password),
    /// so iOS can fill it, offer a strong password and save it in the keychain.
    ///
    /// UI tests launch the app with "-uiTesting": there the simulator's AutoFill
    /// swallows typed keys and shows "Save password?" sheets over the app, so
    /// the hint is left out (nil). Debug builds only; release builds always
    /// keep it.
    static func autoFill(_ type: UITextContentType) -> UITextContentType? {
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains("-uiTesting") { return nil }
        #endif
        return type
    }
}

extension View {
    /// Sets the AutoFill type of a login field (see `UITextContentType.autoFill`).
    func autoFillContentType(_ type: UITextContentType) -> some View {
        textContentType(.autoFill(type))
    }
}
