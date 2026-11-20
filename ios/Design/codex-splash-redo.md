The attached AppIcon.png is approved. Redo only the splash screen files, as an exact match of the icon:

1. SplashLogo.png: 600x600 PNG, TRANSPARENT background. Draw the SAME white house
   with the SAME red heart as in AppIcon.png (same shapes: chimney on the right,
   round heart, not a V), centered, the house about 75% of the width. Nothing
   else: no background square, no gradient, no shadow. The best way is to cut the
   white house and the heart out of AppIcon.png itself (for example with a short
   Swift script using ImageIO/CoreGraphics, or Python standard library only):
   every pixel that is not part of the white house or the heart becomes fully
   transparent, then crop and scale it into the 600x600 canvas.
2. splash-color.txt: one line, the hex color of the middle of the icon gradient
   (a coral), so the splash looks like the icon in full screen.

Check SplashLogo.png with sips (600x600, hasAlpha yes). Do not touch AppIcon.png
or any other file. List the files when done.
