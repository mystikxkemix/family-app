# App icon and splash screen

Made with the Codex CLI image generation, from the briefs in this folder:

1. `codex-brief.md`: the icon (`AppIcon.png`) and a first splash logo.
2. `codex-splash-redo.md`: the splash logo cut out of the approved icon, and the
   splash color (`#E75460`, the middle of the icon gradient).

To make new versions, run Codex in an empty folder with a brief, for example:

```sh
codex exec -s workspace-write -C <folder> "Read BRIEF.md and produce the deliverables it lists."
codex exec -s workspace-write -C <folder> --image=AppIcon.png "Follow the instructions in REDO.md."
```

`--image=` needs the `=`: with a space, Codex reads the prompt as a second image.

Then copy the files into `FamilyApp/Resources/Assets.xcassets`:
`AppIcon.appiconset/AppIcon.png`, `SplashLogo.imageset/SplashLogo.png` (a @3x
image: 600 px = 200 pt) and the color in `LaunchBackground.colorset`.
