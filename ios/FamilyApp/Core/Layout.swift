import SwiftUI

/// Layout helpers for wide screens (iPad full screen or 2/3 Split View).
/// Everything switches on the horizontal size class, never on the device:
/// an iPad in a narrow Split View gets the iPhone layout.

/// The widest a column of text or form rows should be on a big screen.
let readableContentWidth: CGFloat = 720

private struct ReadableContentMargins: ViewModifier {
    @Environment(\.horizontalSizeClass) private var sizeClass
    @State private var width: CGFloat = 0

    func body(content: Content) -> some View {
        content
            .onGeometryChange(for: CGFloat.self) { proxy in
                proxy.size.width
            } action: { newWidth in
                width = newWidth
            }
            // Side margins instead of a narrower frame, so the list background
            // still fills the screen and only the rows are centered.
            .contentMargins(.horizontal, sideMargin, for: .scrollContent)
    }

    private var sideMargin: CGFloat {
        guard sizeClass == .regular else { return 0 }
        return max(0, (width - readableContentWidth) / 2)
    }
}

extension View {
    /// Centers scrollable content (lists, forms, scroll views) in a readable
    /// column on wide screens. No change in compact width.
    func readableContentWidth() -> some View {
        modifier(ReadableContentMargins())
    }
}

/// A toolbar refresh button with ⌘R, shown only on wide screens, where there
/// is often a keyboard or a pointer and pull to refresh is less natural.
struct WideRefreshButton: View {
    let action: () async -> Void
    @Environment(\.horizontalSizeClass) private var sizeClass

    var body: some View {
        if sizeClass == .regular {
            Button("Refresh", systemImage: "arrow.clockwise") {
                Task { await action() }
            }
            .keyboardShortcut("r", modifiers: .command)
        }
    }
}
