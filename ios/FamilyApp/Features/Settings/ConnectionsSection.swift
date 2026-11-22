import SwiftUI

/// The result of one connection test (GET admin/connections).
struct ConnectionResult: Codable, Identifiable, Hashable, Sendable {
    let service: String
    let configured: Bool
    let ok: Bool
    let message: String
    let latencyMs: Int

    var id: String { service }

    /// Display name of the service.
    var name: LocalizedStringResource {
        switch service {
        case "database": "Database"
        case "radarr": "Radarr"
        case "sonarr": "Sonarr"
        case "prowlarr": "Prowlarr"
        case "tmdb": "TMDB"
        case "push": "Push notifications"
        case "jellyfin": "Jellyfin"
        default: LocalizedStringResource(stringLiteral: service)
        }
    }

    var symbol: String {
        switch service {
        case "database": "cylinder.split.1x2"
        case "radarr": "film"
        case "sonarr": "tv"
        case "prowlarr": "magnifyingglass"
        case "tmdb": "info.circle"
        case "push": "bell"
        default: "network"
        }
    }
}

/// Settings > Connections (admins only): tests every outside service the
/// server uses and shows the result.
struct ConnectionsSection: View {
    @Environment(AppSession.self) private var session
    @State private var results: [ConnectionResult] = []
    @State private var isTesting = false

    var body: some View {
        Section {
            ForEach(results) { result in
                ConnectionRow(result: result)
            }
            Button(action: test) {
                HStack {
                    Label("Test again", systemImage: "arrow.clockwise")
                    if isTesting {
                        Spacer()
                        ProgressView()
                    }
                }
            }
            .disabled(isTesting)
            .accessibilityIdentifier("connections.test")
        } header: {
            Text("Connections")
        } footer: {
            Text("Tests the services the server uses. Only admins see this.")
        }
        .task { await run() }
    }

    private func test() {
        Task { await run() }
    }

    private func run() async {
        guard !isTesting else { return }
        isTesting = true
        await session.run { api in
            results = try await api.get("admin/connections")
        }
        isTesting = false
    }
}

private struct ConnectionRow: View {
    let result: ConnectionResult

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Image(systemName: result.symbol)
                .foregroundStyle(.secondary)
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 2) {
                Text(result.name)
                Group {
                    if !result.configured {
                        Text("Not set up")
                    } else if result.ok {
                        Text("\(result.message) · \(result.latencyMs) ms")
                    } else {
                        Text(verbatim: result.message)
                    }
                }
                .font(.caption)
                .foregroundStyle(result.configured && !result.ok ? Color.red : Color.secondary)
                .lineLimit(3)
            }
            Spacer()
            statusIcon
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("connections.\(result.service)")
    }

    @ViewBuilder
    private var statusIcon: some View {
        if !result.configured {
            Image(systemName: "minus.circle").foregroundStyle(.tertiary)
        } else if result.ok {
            Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
        } else {
            Image(systemName: "xmark.circle.fill").foregroundStyle(.red)
        }
    }
}
