import SwiftUI

/// The details page of a watchlist title: TMDB data (synopsis, rating, cast,
/// trailer) on top of our own data (status, who wants it, note, downloads).
struct MediaDetailView: View {
    /// Called after a change, so the watchlist reloads.
    let onChange: () async -> Void

    @Environment(AppSession.self) private var session
    @State private var media: Media
    @State private var state = DetailsState.loading
    @State private var showingEditor = false
    @State private var showingMatch = false
    @State private var downloadMessage: String?

    init(media: Media, onChange: @escaping () async -> Void) {
        _media = State(initialValue: media)
        self.onChange = onChange
    }

    private enum DetailsState {
        case loading
        case loaded(MediaDetails)
        /// The title has no TMDB id yet.
        case notMatched
        /// The server has no TMDB key.
        case notConfigured
        case failed(String)
    }

    private var integrations: Integrations? { session.me?.integrations }

    var body: some View {
        List {
            switch state {
            case .loading:
                Section {
                    HStack {
                        Spacer()
                        ProgressView()
                        Spacer()
                    }
                }
            case .loaded(let details):
                header(details)
                aboutSection(details)
                if !details.cast.isEmpty {
                    castSection(details.cast)
                }
                linksSection(details)
            case .notMatched:
                notMatchedSection
            case .notConfigured:
                notConfiguredSection
            case .failed(let message):
                Section {
                    Label(message, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.secondary)
                    Button("Retry") { Task { await loadDetails() } }
                }
            }

            ourListSection
            toolsSection

            if hasDetails {
                Section {
                    Button("Change match", systemImage: "arrow.triangle.2.circlepath") { showingMatch = true }
                        .accessibilityIdentifier("details.changeMatch")
                }
            }
        }
        .navigationTitle(media.title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            Button("Edit") { showingEditor = true }
                .accessibilityIdentifier("details.edit")
        }
        .sheet(isPresented: $showingEditor) {
            MediaEditor(target: .edit(media)) { await reloadMedia() }
        }
        .sheet(isPresented: $showingMatch) {
            MatchSearchSheet(media: media) { matched in
                media = matched
                await onChange()
                await loadDetails()
            }
        }
        .alert(
            "Download",
            isPresented: Binding(get: { downloadMessage != nil }, set: { if !$0 { downloadMessage = nil } }),
            actions: { Button("OK", role: .cancel) {} },
            message: { Text(downloadMessage ?? "") }
        )
        .refreshable {
            await reloadMedia()
            await loadDetails()
        }
        .task { await loadDetails() }
    }

    // MARK: TMDB sections

    private func header(_ details: MediaDetails) -> some View {
        Section {
            if let backdrop = details.backdropUrl, let url = URL(string: backdrop) {
                AsyncImage(url: url) { image in
                    image.resizable().scaledToFill()
                } placeholder: {
                    Color.secondary.opacity(0.15)
                }
                .frame(height: 180)
                .clipped()
                .listRowInsets(EdgeInsets())
            }
            HStack(alignment: .top, spacing: 14) {
                LargePoster(url: details.posterUrl ?? media.posterUrl, kind: media.kind)
                VStack(alignment: .leading, spacing: 6) {
                    Text(details.title)
                        .font(.title3.bold())
                    if details.originalTitle != details.title && !details.originalTitle.isEmpty {
                        Text(details.originalTitle)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                    Text(factsLine(details))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    if !details.genres.isEmpty {
                        Text(details.genres.formatted(.list(type: .and, width: .narrow)))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    if details.tmdbVotes > 0 {
                        Label {
                            Text("\(details.tmdbRating.formatted(.number.precision(.fractionLength(1))))/10 · \(details.tmdbVotes.formatted(.number.notation(.compactName))) votes")
                        } icon: {
                            Image(systemName: "star.fill").foregroundStyle(.yellow)
                        }
                        .font(.subheadline)
                        .accessibilityIdentifier("details.rating")
                    }
                }
            }
            .padding(.vertical, 4)
        }
    }

    /// "2016 · Movie · 1 hr, 56 min" or "2022 · TV show · 2 seasons · 19 episodes".
    private func factsLine(_ details: MediaDetails) -> String {
        var parts: [String] = []
        if let year = details.year, year > 0 { parts.append(String(year)) }
        parts.append(String(localized: media.kind.title))
        if let minutes = details.runtimeMinutes, minutes > 0 {
            parts.append(Duration.seconds(minutes * 60).formatted(.units(allowed: [.hours, .minutes], width: .abbreviated)))
        }
        if let seasons = details.seasons, seasons > 0 {
            parts.append(String(localized: "\(seasons) seasons"))
        }
        if let episodes = details.episodes, episodes > 0 {
            parts.append(String(localized: "\(episodes) episodes"))
        }
        return parts.joined(separator: " · ")
    }

    private func aboutSection(_ details: MediaDetails) -> some View {
        Section("Overview") {
            if !details.overview.isEmpty {
                Text(details.overview)
                    .accessibilityIdentifier("details.overview")
            }
            if !details.directors.isEmpty {
                let names = details.directors.formatted(.list(type: .and))
                Text(media.kind == .movie ? "Directed by \(names)" : "Created by \(names)")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private func castSection(_ cast: [CastMember]) -> some View {
        Section("Cast") {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(alignment: .top, spacing: 14) {
                    ForEach(Array(cast.enumerated()), id: \.offset) { _, member in
                        CastCard(member: member)
                    }
                }
                .padding(.vertical, 4)
            }
            .listRowInsets(EdgeInsets(top: 8, leading: 16, bottom: 8, trailing: 16))
        }
    }

    private func linksSection(_ details: MediaDetails) -> some View {
        Section {
            if let trailer = details.trailerUrl, let url = URL(string: trailer) {
                Link(destination: url) {
                    Label("Trailer", systemImage: "play.rectangle")
                }
            }
            if let url = URL(string: details.tmdbUrl) {
                Link(destination: url) {
                    Label("Open on TMDB", systemImage: "safari")
                }
            }
        }
    }

    private var notMatchedSection: some View {
        Section {
            Label("No details yet", systemImage: "film")
                .font(.headline)
            Text("This title is not linked to TMDB yet.")
                .foregroundStyle(.secondary)
            Button("Find the details", systemImage: "magnifyingglass") { showingMatch = true }
                .accessibilityIdentifier("details.findDetails")
        }
    }

    @ViewBuilder
    private var notConfiguredSection: some View {
        if session.me?.user.isAdmin == true {
            Section {
                Text("Add a TMDB key in the Admin page of the website to see details.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Our data

    /// True when the TMDB header already shows the poster and title.
    private var hasDetails: Bool {
        if case .loaded = state { return true }
        return false
    }

    private var ourListSection: some View {
        Section("Our list") {
            if !hasDetails {
                HStack(spacing: 12) {
                    PosterView(url: media.posterUrl, kind: media.kind)
                    VStack(alignment: .leading) {
                        Text(media.title).font(.headline)
                        if let year = media.year, year > 0 {
                            Text(String(year)).foregroundStyle(.secondary)
                        }
                    }
                }
            }
            Picker("Status", selection: Binding(get: { media.status }, set: { update(["status": .string($0.rawValue)]) })) {
                ForEach(MediaStatus.allCases) { status in
                    Text(status.title).tag(status)
                }
            }
            interestButton
            InterestLine(userIds: media.interestedUserIds)
            if !media.note.isEmpty {
                Text(media.note)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var interestButton: some View {
        let interested = media.interestedUserIds.contains(session.me?.user.id ?? "")
        return Button(
            interested ? LocalizedStringKey("Not for me") : LocalizedStringKey("I want to watch it"),
            systemImage: interested ? "heart.slash" : "heart"
        ) {
            update(["interested": .bool(!interested)])
        }
        .tint(.pink)
    }

    @ViewBuilder
    private var toolsSection: some View {
        let library = integrations?.library(for: media.kind) ?? false
        let prowlarr = integrations?.prowlarr ?? false
        if library || prowlarr {
            Section {
                if library {
                    Button(media.kind == .movie ? LocalizedStringKey("Download with Radarr") : LocalizedStringKey("Download with Sonarr"),
                           systemImage: "arrow.down.circle", action: download)
                }
                if prowlarr {
                    NavigationLink {
                        ReleasesView(kind: media.kind, initialQuery: releaseQuery)
                    } label: {
                        Label("Search releases (Prowlarr)", systemImage: "magnifyingglass")
                    }
                }
            } footer: {
                if media.downloadRequestedAt != nil {
                    Text("Download already requested.")
                }
            }
        }
    }

    private var releaseQuery: String {
        if let year = media.year, year > 0, media.kind == .movie {
            return "\(media.title) \(year)"
        }
        return media.title
    }

    // MARK: Actions

    /// Loads the TMDB details. "Not linked" and "no TMDB key" are normal
    /// states of the page, not errors to show in an alert.
    private func loadDetails() async {
        guard let api = session.api else { return }
        do {
            let details: MediaDetails = try await api.get("watchlist/\(media.id)/details", query: [AppLanguage.queryItem])
            state = .loaded(details)
        } catch is CancellationError {
            return
        } catch APIError.server(_, "not_matched", _) {
            state = .notMatched
        } catch APIError.server(_, "not_configured", _) {
            state = .notConfigured
        } catch APIError.unauthorized {
            // Let the session log out, as for any other call.
            await session.run { _ in throw APIError.unauthorized }
        } catch APIError.server(_, "password_change_required", let message) {
            await session.run { _ in throw APIError.server(status: 403, code: "password_change_required", message: message) }
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    private func reloadMedia() async {
        await session.run { api in
            let all: [Media] = try await api.get("watchlist")
            if let fresh = all.first(where: { $0.id == media.id }) {
                media = fresh
            }
        }
        await onChange()
    }

    private func update(_ body: JSONBody) {
        Task {
            let saved = await session.run { api in
                let updated: Media = try await api.patch("watchlist/\(media.id)", body)
                withAnimation { media = updated }
            }
            if saved { await onChange() }
        }
    }

    private func download() {
        Task {
            await session.run { api in
                let response: DownloadResponse = try await api.post("watchlist/\(media.id)/download")
                media = response.media
                downloadMessage = response.alreadyInLibrary
                    ? String(localized: "This title is already in the library.")
                    : String(localized: "Sent. The download will start soon.")
            }
            await onChange()
        }
    }
}

/// A bigger poster for the details page.
private struct LargePoster: View {
    let url: String?
    let kind: MediaKind

    var body: some View {
        Group {
            if let url, let imageURL = URL(string: url) {
                AsyncImage(url: imageURL) { image in
                    image.resizable().scaledToFill()
                } placeholder: {
                    Color.secondary.opacity(0.15)
                }
            } else {
                Image(systemName: kind.symbol)
                    .font(.largeTitle)
                    .foregroundStyle(.tint)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(Color.secondary.opacity(0.1))
            }
        }
        .frame(width: 100, height: 150)
        .clipShape(RoundedRectangle(cornerRadius: 8))
    }
}

/// One actor: photo, name and role.
private struct CastCard: View {
    let member: CastMember

    var body: some View {
        VStack(spacing: 4) {
            Group {
                if let photo = member.photoUrl, let url = URL(string: photo) {
                    AsyncImage(url: url) { image in
                        image.resizable().scaledToFill()
                    } placeholder: {
                        Color.secondary.opacity(0.15)
                    }
                } else {
                    Image(systemName: "person.fill")
                        .font(.title)
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                        .background(Color.secondary.opacity(0.1))
                }
            }
            .frame(width: 64, height: 64)
            .clipShape(Circle())
            Text(member.name)
                .font(.caption.bold())
                .lineLimit(2)
                .multilineTextAlignment(.center)
            Text(member.character)
                .font(.caption2)
                .foregroundStyle(.secondary)
                .lineLimit(2)
                .multilineTextAlignment(.center)
        }
        .frame(width: 84)
    }
}

/// Search sheet to link a title to the right TMDB entry ("Find the details"
/// and "Change match").
struct MatchSearchSheet: View {
    let media: Media
    let onMatched: (Media) async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var query = ""
    @State private var results: [MediaSearchResult] = []
    @State private var isSearching = false
    @State private var hasSearched = false
    @State private var isSaving = false

    var body: some View {
        NavigationStack {
            List {
                Section {
                    HStack {
                        TextField("Search a title", text: $query)
                            .submitLabel(.search)
                            .onSubmit { Task { await search() } }
                        if isSearching {
                            ProgressView()
                        } else {
                            Button("Search", systemImage: "magnifyingglass") { Task { await search() } }
                                .labelStyle(.iconOnly)
                                .disabled(query.trimmingCharacters(in: .whitespaces).isEmpty)
                        }
                    }
                }
                ForEach(results) { result in
                    Button {
                        Task { await choose(result) }
                    } label: {
                        SearchResultRow(result: result)
                    }
                    .disabled(result.externalIds.tmdb == nil || isSaving)
                }
            }
            .navigationTitle("Pick the right title")
            .navigationBarTitleDisplayMode(.inline)
            .overlay {
                if hasSearched && !isSearching && results.isEmpty {
                    ContentUnavailableView.search(text: query)
                }
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
            .task {
                if query.isEmpty {
                    query = media.title
                    await search()
                }
            }
        }
    }

    private func search() async {
        let term = query.trimmingCharacters(in: .whitespaces)
        guard !term.isEmpty else { return }
        isSearching = true
        await session.run { api in
            results = try await api.get("search/media", query: [
                URLQueryItem(name: "kind", value: media.kind.rawValue),
                URLQueryItem(name: "q", value: term),
                AppLanguage.queryItem,
            ])
        }
        isSearching = false
        hasSearched = true
    }

    private func choose(_ result: MediaSearchResult) async {
        guard let tmdbId = result.externalIds.tmdb else { return }
        isSaving = true
        var matched: Media?
        await session.run { api in
            matched = try await api.post("watchlist/\(media.id)/match", ["tmdbId": .int(tmdbId)])
        }
        isSaving = false
        if let matched {
            await onMatched(matched)
            dismiss()
        }
    }
}

/// One search result: poster, title, year, "already in the library".
struct SearchResultRow: View {
    let result: MediaSearchResult

    var body: some View {
        HStack(spacing: 12) {
            PosterView(url: result.posterUrl, kind: result.kind)
            VStack(alignment: .leading) {
                Text(result.title).foregroundStyle(.primary)
                if let year = result.year, year > 0 {
                    Text(String(year)).font(.caption).foregroundStyle(.secondary)
                }
                if result.inLibrary {
                    Text("Already in the library").font(.caption).foregroundStyle(.green)
                }
            }
        }
    }
}
