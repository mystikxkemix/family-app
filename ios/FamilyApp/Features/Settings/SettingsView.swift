import SwiftUI
import UserNotifications

/// Profile, household, invites, notifications and logout.
struct SettingsView: View {
    @Environment(AppSession.self) private var session
    @State private var displayName = ""
    @State private var householdName = ""
    @State private var invite: InviteResponse?
    @State private var notificationStatus = UNAuthorizationStatus.notDetermined
    @State private var showingPasswordSheet = false
    @State private var confirmingLogout = false

    var body: some View {
        NavigationStack {
            Form {
                profileSection
                householdSection
                inviteSection
                notificationSection
                accountSection
                aboutSection
            }
            .navigationTitle("Settings")
            .task {
                fillFields()
                notificationStatus = await PushManager.shared.authorizationStatus()
            }
            .refreshable {
                await session.refreshMe()
                fillFields()
            }
            .sheet(isPresented: $showingPasswordSheet) { PasswordSheet() }
        }
    }

    // MARK: Sections

    private var profileSection: some View {
        Section("You") {
            HStack {
                TextField("Your name", text: $displayName)
                if displayName != (session.me?.user.displayName ?? "") && !displayName.isEmpty {
                    Button("Save") {
                        Task {
                            await session.run { api in
                                let _: User = try await api.patch("me", ["displayName": .string(displayName)])
                            }
                            await session.refreshMe()
                        }
                    }
                }
            }
            if let username = session.me?.user.username {
                LabeledContent("Username", value: username)
            }
        }
    }

    private var householdSection: some View {
        Section("Household") {
            HStack {
                TextField("Household name", text: $householdName)
                if householdName != (session.me?.household.name ?? "") && !householdName.isEmpty {
                    Button("Save") {
                        Task {
                            await session.run { api in
                                let _: Household = try await api.patch("household", ["name": .string(householdName)])
                            }
                            await session.refreshMe()
                        }
                    }
                }
            }
            ForEach(session.me?.members ?? []) { member in
                Label(member.displayName, systemImage: member.id == session.me?.user.id ? "person.fill" : "person")
            }
        }
    }

    private var inviteSection: some View {
        Section {
            if let invite {
                LabeledContent("Invite code") {
                    Text(invite.code)
                        .font(.body.monospaced())
                        .accessibilityIdentifier("settings.inviteCode")
                        .textSelection(.enabled)
                }
                Text("Valid until \(invite.expiresAt.formatted(date: .abbreviated, time: .shortened)), for one person.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                ShareLink(item: inviteMessage(invite)) {
                    Label("Share the invite", systemImage: "square.and.arrow.up")
                }
            } else {
                Button("Invite someone", systemImage: "person.badge.plus") {
                    Task {
                        await session.run { api in
                            invite = try await api.post("household/invites")
                        }
                    }
                }
                .accessibilityIdentifier("settings.invite")
            }
        } header: {
            Text("Invite")
        } footer: {
            Text("The other person installs the app, chooses Join and types this code.")
        }
    }

    private var notificationSection: some View {
        Section {
            switch notificationStatus {
            case .authorized, .provisional, .ephemeral:
                Label("Notifications are on", systemImage: "bell.badge")
            case .denied:
                Label("Notifications are off", systemImage: "bell.slash")
                Button("Open iPhone settings") {
                    if let url = URL(string: UIApplication.openSettingsURLString) {
                        UIApplication.shared.open(url)
                    }
                }
            default:
                Button("Turn on notifications", systemImage: "bell") {
                    Task {
                        _ = await PushManager.shared.requestPermission(session: session)
                        notificationStatus = await PushManager.shared.authorizationStatus()
                    }
                }
            }
        } header: {
            Text("Notifications")
        } footer: {
            if session.me?.pushEnabled == false {
                Text("The server has no push key yet, so it cannot send notifications.")
            } else {
                Text("You get a notification when someone else adds something.")
            }
        }
    }

    private var accountSection: some View {
        Section {
            Button("Change password", systemImage: "key") { showingPasswordSheet = true }
            Button("Log out", systemImage: "rectangle.portrait.and.arrow.right", role: .destructive) {
                confirmingLogout = true
            }
            .confirmationDialog("Log out?", isPresented: $confirmingLogout, titleVisibility: .visible) {
                Button("Log out", role: .destructive) {
                    Task { await session.logout() }
                }
            }
        }
    }

    private var aboutSection: some View {
        Section("About") {
            if let server = session.serverURL {
                LabeledContent("Server", value: server.host() ?? server.absoluteString)
            }
            LabeledContent("Version", value: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?")
            // Attribution asked by the TMDB API terms.
            Text("This product uses the TMDB API but is not endorsed or certified by TMDB.")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    // MARK: Helpers

    private func fillFields() {
        displayName = session.me?.user.displayName ?? ""
        householdName = session.me?.household.name ?? ""
    }

    private func inviteMessage(_ invite: InviteResponse) -> String {
        let server = session.serverURL?.absoluteString ?? ""
        return String(localized: "Join our household in the Family app.\nServer: \(server)\nInvite code: \(invite.code)")
    }
}

/// Changes the password. The other phones of this account get logged out.
private struct PasswordSheet: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var current = ""
    @State private var new = ""
    @State private var confirmation = ""

    private var isValid: Bool { !current.isEmpty && new.count >= 8 && new == confirmation }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    SecureField("Current password", text: $current)
                        .autoFillContentType(.password)
                }
                Section {
                    SecureField("New password", text: $new)
                        .autoFillContentType(.newPassword)
                    SecureField("Confirm new password", text: $confirmation)
                        .autoFillContentType(.newPassword)
                } footer: {
                    Text("At least 8 characters. Your other devices will be logged out.")
                }
            }
            .navigationTitle("Change password")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        Task {
                            if await session.changePassword(current: current, new: new) { dismiss() }
                        }
                    }
                    .disabled(!isValid)
                }
            }
        }
    }
}
