import SwiftUI

@main
struct FamilyApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var session = AppSession()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(session)
                .task {
                    if session.isLoggedIn {
                        await session.refreshMe()
                        await PushManager.shared.registerIfAllowed(session: session)
                    }
                }
                .onChange(of: scenePhase) { _, phase in
                    // Coming back to the app: the other person may have changed things.
                    if phase == .active && session.isLoggedIn {
                        NotificationCenter.default.post(name: .familyDataChanged, object: nil)
                    }
                }
        }
    }
}

/// Shows the login screen or the app, and the error alerts of both.
struct RootView: View {
    @Environment(AppSession.self) private var session

    var body: some View {
        @Bindable var session = session
        Group {
            if session.isLoggedIn && session.passwordChangeRequired {
                ChoosePasswordView()
            } else if session.isLoggedIn {
                MainTabView()
            } else {
                AuthView()
            }
        }
        .alert(
            "Something went wrong",
            isPresented: Binding(
                get: { session.errorMessage != nil },
                set: { if !$0 { session.errorMessage = nil } }
            ),
            actions: { Button("OK", role: .cancel) {} },
            message: { Text(session.errorMessage ?? "") }
        )
    }
}

struct MainTabView: View {
    var body: some View {
        // A tab bar on iPhone; a sidebar (that can turn into a tab bar) on iPad.
        // In a narrow Split View, the iPad gets the iPhone tab bar.
        TabView {
            Tab("Lists", systemImage: "checklist") { ListsView() }
            Tab("Watchlist", systemImage: "popcorn") { WatchlistView() }
            Tab("Places", systemImage: "fork.knife") { PlacesView() }
            Tab("Events", systemImage: "calendar") { EventsView() }
            Tab("Settings", systemImage: "gearshape") { SettingsView().readableContentWidth() }
        }
        .tabViewStyle(.sidebarAdaptable)
    }
}

extension View {
    /// Reloads when shared data may have changed (push, app reopened).
    func onFamilyDataChanged(_ action: @escaping @MainActor () async -> Void) -> some View {
        onReceive(NotificationCenter.default.publisher(for: .familyDataChanged)) { _ in
            Task { await action() }
        }
    }
}
