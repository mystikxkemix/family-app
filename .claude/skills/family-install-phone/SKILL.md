---
name: family-install-phone
description: Build the Family iPhone app and install it on Mike's or his partner's iPhone with the right signing team. Use when asked to install, build on the phone, or update the app on a device.
---

# Install on a phone

Profiles, device ids and teams: CLAUDE.local.md. Background: `.claude/docs/ios.md`.

1. Find the phone: `xcrun devicectl list devices` ("available (paired)" works
   over Wi-Fi; "unavailable" means asleep or away: ask Mike to unlock it).
2. Make sure `ios/Config/Signing/<profile>.xcconfig` exists (gitignored).
   Without an Apple account in Xcode, it must contain
   `CODE_SIGN_ENTITLEMENTS = FamilyApp/Resources/FamilyApp-NoPush.entitlements`
   (the team wildcard profile has no push).
3. Install: `ios/Scripts/install.sh <profile> <device-id>`.
4. Optional launch: `xcrun devicectl device process launch --device <id> <bundle-id>`
   ("Locked" = the phone is locked, not a bug).
5. A new team or bundle id: the phone must be registered in that team, and the
   first build may need Xcode signed in to that team to create the profile.
