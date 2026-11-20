import Foundation

// The JSON shapes of the API (see api/internal/store/models.go).

struct User: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var username: String
    var displayName: String
    let createdAt: Date
}

struct Household: Codable, Hashable, Sendable {
    let id: String
    var name: String
    let createdAt: Date
}

struct MeResponse: Codable, Sendable {
    var user: User
    var household: Household
    var members: [User]
    /// False when the server has no APNs key at all.
    var pushEnabled: Bool
}

struct AuthResponse: Codable, Sendable {
    let token: String
    let user: User
}

struct InviteResponse: Codable, Sendable {
    let code: String
    let expiresAt: Date
}

struct DeviceRegistration: Codable, Sendable {
    /// False when the server has no APNs key for this app's bundle identifier.
    let supported: Bool
}

// MARK: - Lists

enum ListKind: String, Codable, CaseIterable, Identifiable, Sendable {
    case grocery, todo, other

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .grocery: "Groceries"
        case .todo: "To-do"
        case .other: "Other"
        }
    }

    var defaultEmoji: String {
        switch self {
        case .grocery: "🛒"
        case .todo: "✅"
        case .other: "📝"
        }
    }
}

struct SharedList: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var name: String
    var kind: ListKind
    var emoji: String
    let createdBy: String
    let createdAt: Date
    var updatedAt: Date
    var openCount: Int
    var totalCount: Int

    var displayEmoji: String { emoji.isEmpty ? kind.defaultEmoji : emoji }
}

struct ListItem: Codable, Identifiable, Hashable, Sendable {
    let id: String
    let listId: String
    var text: String
    var note: String
    var checked: Bool
    var checkedBy: String?
    var checkedAt: Date?
    let createdBy: String
    let createdAt: Date
    var updatedAt: Date
}

// MARK: - Watchlist

enum MediaKind: String, Codable, CaseIterable, Identifiable, Sendable {
    case movie, show

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .movie: "Movie"
        case .show: "TV show"
        }
    }

    var symbol: String {
        switch self {
        case .movie: "film"
        case .show: "tv"
        }
    }
}

enum MediaStatus: String, Codable, CaseIterable, Identifiable, Sendable {
    case toWatch, watching, watched

    var id: String { rawValue }

    var title: LocalizedStringResource {
        switch self {
        case .toWatch: "To watch"
        case .watching: "Watching"
        case .watched: "Watched"
        }
    }
}

struct ExternalIDs: Codable, Hashable, Sendable {
    var tmdb: Int?
    var tvdb: Int?
    var imdb: String?
}

struct Media: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var kind: MediaKind
    var title: String
    var year: Int?
    var posterUrl: String?
    var overview: String?
    var note: String
    var status: MediaStatus
    var interestedUserIds: [String]
    var externalIds: ExternalIDs
    let addedBy: String
    var watchedAt: Date?
    let createdAt: Date
    var updatedAt: Date
}

// MARK: - Events

/// A shared calendar entry. Named FamilyEvent to avoid confusion with EventKit.
struct FamilyEvent: Codable, Identifiable, Hashable, Sendable {
    let id: String
    var title: String
    var startsAt: Date
    var endsAt: Date?
    var allDay: Bool
    var location: String
    var notes: String
    let createdBy: String
    let createdAt: Date
    var updatedAt: Date
}
