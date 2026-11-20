import Foundation
import Observation

extension Notification.Name {
    /// Posted when shared data may have changed (push received, app reopened).
    /// Screens reload when they get it.
    static let familyDataChanged = Notification.Name("familyDataChanged")
}

/// The logged-in state of the app: server address, token and profile.
@MainActor
@Observable
final class AppSession {
    private static let serverKey = "serverURL"
    private static let tokenKey = "token"

    private(set) var serverURL: URL?
    private(set) var token: String?
    /// The user, the household and its members. Nil until loaded.
    private(set) var me: MeResponse?
    /// The last error, shown as an alert by the root view.
    var errorMessage: String?

    var isLoggedIn: Bool { token != nil && serverURL != nil }

    /// A client for the current server and token.
    var api: APIClient? {
        guard let serverURL else { return nil }
        return APIClient(baseURL: serverURL, token: token)
    }

    init() {
        #if DEBUG
        // UI tests start from a logged-out app. Debug builds only.
        if ProcessInfo.processInfo.arguments.contains("-uiTestReset") {
            Keychain.delete(Self.tokenKey)
            UserDefaults.standard.removeObject(forKey: Self.serverKey)
        }
        #endif
        serverURL = UserDefaults.standard.url(forKey: Self.serverKey)
        token = Keychain.read(Self.tokenKey)
    }

    // MARK: Login

    func login(server: String, username: String, password: String) async throws {
        let url = try APIClient.serverURL(from: server)
        let response: AuthResponse = try await APIClient(baseURL: url, token: nil)
            .post("auth/login", ["username": .string(username), "password": .string(password)])
        await start(url: url, response: response)
    }

    enum Signup {
        case invite(code: String)
        case setup(code: String, householdName: String)
    }

    func register(server: String, username: String, password: String, displayName: String, signup: Signup) async throws {
        let url = try APIClient.serverURL(from: server)
        var body: JSONBody = [
            "username": .string(username),
            "password": .string(password),
            "displayName": .string(displayName),
        ]
        switch signup {
        case .invite(let code):
            body["inviteCode"] = .string(code)
        case .setup(let code, let householdName):
            body["setupCode"] = .string(code)
            body["householdName"] = .string(householdName)
        }
        let response: AuthResponse = try await APIClient(baseURL: url, token: nil).post("auth/register", body)
        await start(url: url, response: response)
    }

    private func start(url: URL, response: AuthResponse) async {
        UserDefaults.standard.set(url, forKey: Self.serverKey)
        Keychain.write(response.token, for: Self.tokenKey)
        serverURL = url
        token = response.token
        await refreshMe()
        await PushManager.shared.registerIfAllowed(session: self)
    }

    /// Logs out on the server too, so the token cannot be used any more.
    func logout() async {
        await PushManager.shared.unregister(session: self)
        if let api {
            _ = try? await api.post("auth/logout") as NoContent
        }
        clearLogin()
    }

    private func clearLogin() {
        Keychain.delete(Self.tokenKey)
        token = nil
        me = nil
    }

    // MARK: Profile

    func refreshMe() async {
        await run { api in
            self.me = try await api.get("me")
        }
    }

    func member(id: String) -> User? {
        me?.members.first { $0.id == id }
    }

    func memberName(id: String) -> String {
        member(id: id)?.displayName ?? String(localized: "Someone")
    }

    // MARK: Errors

    /// Runs an API call, shows its error as an alert, and logs out when the
    /// session expired. Returns false when the call failed.
    @discardableResult
    func run(_ work: (APIClient) async throws -> Void) async -> Bool {
        guard let api else { return false }
        do {
            try await work(api)
            return true
        } catch is CancellationError {
            return false
        } catch let error as APIError {
            if error == .unauthorized { clearLogin() }
            errorMessage = error.localizedDescription
            return false
        } catch {
            errorMessage = error.localizedDescription
            return false
        }
    }
}
