import UIKit
import UserNotifications

/// Asks for notification permission, sends the device token to the API, and
/// reloads the screens when a notification arrives.
@MainActor
final class PushManager: NSObject {
    static let shared = PushManager()

    /// The APNs device token as hex, once iOS gave it.
    private(set) var deviceToken: String?
    private weak var session: AppSession?

    /// Whether the user allowed notifications, for the settings screen.
    func authorizationStatus() async -> UNAuthorizationStatus {
        await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
    }

    /// Registers for pushes when the user already allowed them. Called at
    /// launch and after login; never shows the permission prompt.
    func registerIfAllowed(session: AppSession) async {
        self.session = session
        guard session.isLoggedIn else { return }
        let status = await authorizationStatus()
        if status == .authorized || status == .provisional {
            UIApplication.shared.registerForRemoteNotifications()
        }
    }

    /// Shows the permission prompt, then registers. Returns true when allowed.
    func requestPermission(session: AppSession) async -> Bool {
        self.session = session
        let granted = (try? await UNUserNotificationCenter.current()
            .requestAuthorization(options: [.alert, .sound, .badge])) ?? false
        if granted {
            UIApplication.shared.registerForRemoteNotifications()
        }
        return granted
    }

    /// Called by the app delegate with the token from APNs.
    func didRegister(deviceToken data: Data) {
        let token = data.map { String(format: "%02x", $0) }.joined()
        deviceToken = token
        guard let session, session.isLoggedIn else { return }
        Task { await self.upload(token: token, session: session) }
    }

    private func upload(token: String, session: AppSession) async {
        let topic = Bundle.main.bundleIdentifier ?? ""
        // Builds installed from Xcode talk to the APNs sandbox; TestFlight and
        // ad hoc builds to production. The build setting tells which.
        let apsEnvironment = Bundle.main.object(forInfoDictionaryKey: "APSEnvironment") as? String
        let environment = apsEnvironment == "production" ? "production" : "sandbox"
        await session.run { api in
            let _: DeviceRegistration = try await api.put(
                "devices/\(token)", ["topic": .string(topic), "environment": .string(environment)])
        }
    }

    /// Stops pushes to this phone for the current account (before logout).
    func unregister(session: AppSession) async {
        guard let deviceToken, let api = session.api else { return }
        try? await api.delete("devices/\(deviceToken)")
    }
}

extension PushManager: UNUserNotificationCenterDelegate {
    /// Shows the banner even when the app is open, and reloads the data.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        await MainActor.run {
            NotificationCenter.default.post(name: .familyDataChanged, object: nil)
        }
        return [.banner, .list, .sound]
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse
    ) async {
        await MainActor.run {
            NotificationCenter.default.post(name: .familyDataChanged, object: nil)
        }
    }
}

/// UIKit entry points that SwiftUI does not offer.
final class AppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = PushManager.shared
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        PushManager.shared.didRegister(deviceToken: deviceToken)
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        // Happens on the simulator and with builds that lack the push
        // capability. The app works without pushes.
        print("Push registration failed: \(error.localizedDescription)")
    }
}
