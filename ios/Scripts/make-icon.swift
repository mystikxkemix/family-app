// Draws the 1024x1024 app icon: a warm gradient with a white house and a heart.
// Pure CoreGraphics (no AppKit drawing), so it also runs in a headless shell.
// Run from ios/: swift Scripts/make-icon.swift
import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

let size = 1024
let space = CGColorSpace(name: CGColorSpace.sRGB)!
let context = CGContext(data: nil, width: size, height: size, bitsPerComponent: 8, bytesPerRow: 0,
                        space: space, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!

let orange = CGColor(srgbRed: 1.0, green: 0.55, blue: 0.40, alpha: 1)
let pink = CGColor(srgbRed: 0.90, green: 0.30, blue: 0.42, alpha: 1)
let white = CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 1)

// Background: diagonal gradient (CoreGraphics has y going up).
let gradient = CGGradient(colorsSpace: space, colors: [orange, pink] as CFArray, locations: [0, 1])!
context.drawLinearGradient(gradient, start: CGPoint(x: 0, y: 1024), end: CGPoint(x: 1024, y: 0), options: [])

// House: roof triangle + body, with rounded joins.
context.setFillColor(white)
context.setStrokeColor(white)
context.setLineJoin(.round)
context.setLineWidth(60)
let roof = CGMutablePath()
roof.move(to: CGPoint(x: 512, y: 820))
roof.addLine(to: CGPoint(x: 190, y: 530))
roof.addLine(to: CGPoint(x: 834, y: 530))
roof.closeSubpath()
context.addPath(roof)
context.drawPath(using: .fillStroke)
context.addPath(CGPath(roundedRect: CGRect(x: 270, y: 210, width: 484, height: 360), cornerWidth: 48, cornerHeight: 48, transform: nil))
context.fillPath()

// Heart in the middle of the house.
let heart = CGMutablePath()
let cx = 512.0, cy = 400.0, r = 62.0
heart.move(to: CGPoint(x: cx, y: cy - 2.1 * r))
heart.addCurve(to: CGPoint(x: cx - 2 * r, y: cy + 0.4 * r), control1: CGPoint(x: cx - 0.6 * r, y: cy - 1.5 * r), control2: CGPoint(x: cx - 2 * r, y: cy - 0.8 * r))
heart.addArc(center: CGPoint(x: cx - r, y: cy + 0.4 * r), radius: r, startAngle: .pi, endAngle: 0, clockwise: true)
heart.addArc(center: CGPoint(x: cx + r, y: cy + 0.4 * r), radius: r, startAngle: .pi, endAngle: 0, clockwise: true)
heart.addCurve(to: CGPoint(x: cx, y: cy - 2.1 * r), control1: CGPoint(x: cx + 2 * r, y: cy - 0.8 * r), control2: CGPoint(x: cx + 0.6 * r, y: cy - 1.5 * r))
context.setFillColor(pink)
context.addPath(heart)
context.fillPath()

let url = URL(fileURLWithPath: "FamilyApp/Resources/Assets.xcassets/AppIcon.appiconset/AppIcon.png")
let destination = CGImageDestinationCreateWithURL(url as CFURL, UTType.png.identifier as CFString, 1, nil)!
CGImageDestinationAddImage(destination, context.makeImage()!, nil)
CGImageDestinationFinalize(destination)
print("icon written")
