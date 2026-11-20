import SwiftUI

/// Login, or joining a household with an invite code. The very first account
/// is the admin set on the server (ADMIN_USERNAME / ADMIN_PASSWORD): it logs in
/// here, then must choose a new password.
struct AuthView: View {
    @Environment(AppSession.self) private var session

    private enum Mode: Hashable {
        case login, join
    }

    @State private var mode = Mode.login
    @State private var server = ""
    @State private var username = ""
    @State private var password = ""
    @State private var displayName = ""
    @State private var code = ""
    @State private var isWorking = false
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    // Verbatim: a localized string turns the URL into a blue Markdown link.
                    TextField("Server", text: $server, prompt: Text(verbatim: "https://family.example.com"))
                        .accessibilityIdentifier("auth.server")
                        .keyboardType(.URL)
                        .textContentType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                } header: {
                    Text("Server")
                } footer: {
                    Text("The address of your Family API.")
                }

                Section {
                    Picker("Mode", selection: $mode) {
                        Text("Log in").tag(Mode.login)
                        Text("Join").tag(Mode.join)
                    }
                    .pickerStyle(.segmented)
                    .accessibilityIdentifier("auth.mode")
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }

                Section {
                    TextField("Username", text: $username)
                        .accessibilityIdentifier("auth.username")
                        .autoFillContentType(.username)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    SecureField("Password", text: $password)
                        .accessibilityIdentifier("auth.password")
                        .autoFillContentType(mode == .login ? .password : .newPassword)
                    if mode != .login {
                        TextField("Your first name", text: $displayName)
                            .accessibilityIdentifier("auth.displayName")
                            .textContentType(.givenName)
                    }
                } footer: {
                    if mode == .login {
                        Text("First time? Log in with the admin account set on the server, then choose your own password.")
                    } else {
                        Text("At least 8 characters for the password.")
                    }
                }

                if mode == .join {
                    Section {
                        TextField("Invite code", text: $code)
                            .accessibilityIdentifier("auth.code")
                            .textInputAutocapitalization(.characters)
                            .autocorrectionDisabled()
                    } footer: {
                        Text("Ask a member of the household to create an invite in Settings.")
                    }
                }

                if let errorMessage {
                    Section {
                        Label(errorMessage, systemImage: "exclamationmark.triangle")
                            .foregroundStyle(.red)
                    }
                }

                Section {
                    Button(action: submit) {
                        HStack {
                            Spacer()
                            if isWorking {
                                ProgressView()
                            } else {
                                Text(submitTitle).bold()
                            }
                            Spacer()
                        }
                    }
                    .accessibilityIdentifier("auth.submit")
                    .disabled(!canSubmit || isWorking)
                }
            }
            .navigationTitle("Welcome")
            .onAppear {
                if server.isEmpty, let saved = session.serverURL {
                    server = saved.absoluteString
                }
            }
        }
    }

    private var submitTitle: LocalizedStringKey {
        mode == .login ? "Log in" : "Create my account"
    }

    private var canSubmit: Bool {
        let base = !server.isEmpty && !username.isEmpty && !password.isEmpty
        switch mode {
        case .login: return base
        case .join: return base && !displayName.isEmpty && !code.isEmpty
        }
    }

    private func submit() {
        isWorking = true
        errorMessage = nil
        Task {
            defer { isWorking = false }
            do {
                switch mode {
                case .login:
                    try await session.login(server: server, username: username, password: password)
                case .join:
                    try await session.register(
                        server: server, username: username, password: password,
                        displayName: displayName, inviteCode: code)
                }
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }
}
