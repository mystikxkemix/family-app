# Brief: app icon and splash screen for "Family" (French: "Famille")

An iPhone app for a couple / household to share things: grocery and to-do lists,
a watchlist of movies and shows, places to try (restaurants, bars), and events.
Warm, friendly, private, "our home". Not childish, not corporate.

Brand color: warm coral #F26E57 (accent color of the app), with a deeper
pink-red #E64D6B. Light, soft, modern iOS look.

Make original artwork. No text, no letters, no logos of other brands.

## Deliverables (save exactly these files in this folder)

1. `AppIcon.png`: 1024x1024 PNG, square, full bleed, NO transparency, NO rounded
   corners (iOS adds the mask), no drop shadow around the square. One simple,
   bold symbol that reads well at 60x60: for example a small house and a heart,
   or two shapes sharing one roof. Background: a soft coral gradient.
2. `SplashLogo.png`: the same symbol alone, centered, on a TRANSPARENT
   background, 600x600 PNG. It is shown centered on a plain background color at
   app launch, so no background shape and generous empty margin (symbol about
   60% of the width).
3. `splash-color.txt`: one line, the hex color of the launch background that
   matches the logo (for example a very light warm cream, or the coral itself
   if the logo is white).

Check each file with `sips -g pixelWidth -g pixelHeight -g hasAlpha <file>` and
fix it if the size or the alpha is wrong (resize with `sips -z`). Do not edit or
create any other file outside this folder.
