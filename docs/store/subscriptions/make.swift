// Draws the two subscription images (1024 x 1024, RGB, no alpha, square corners: App Store Connect's rules for a
// subscription's promotional image) in the style of the app icon: the same blue, the same phone and waves.
// Run: swift make.swift   (writes camera-monthly.png and camera-6-months.png next to this file)
import AppKit

let size: CGFloat = 1024

func draw(title: String, period: String, to path: String) {
  // An opaque RGB bitmap (no alpha channel at all), drawn with Core Graphics.
  let cg = CGContext(
    data: nil, width: Int(size), height: Int(size), bitsPerComponent: 8, bytesPerRow: 0,
    space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
  // Work with the origin at the top left, like the icon's own coordinates.
  cg.translateBy(x: 0, y: size)
  cg.scaleBy(x: 1, y: -1)

  // The icon's background: a blue that gets slightly darker towards the bottom (the icon's own pixel values).
  let space = CGColorSpace(name: CGColorSpace.sRGB)!
  let white = CGColor(colorSpace: space, components: [1, 1, 1, 1])!
  let gradient = CGGradient(
    colorsSpace: space,
    colors: [
      CGColor(colorSpace: space, components: [58 / 255, 106 / 255, 254 / 255, 1])!,
      CGColor(colorSpace: space, components: [37 / 255, 75 / 255, 224 / 255, 1])!
    ] as CFArray, locations: [0, 1])!
  cg.drawLinearGradient(
    gradient, start: CGPoint(x: 0, y: 0), end: CGPoint(x: 0, y: size), options: [])

  // The icon's glyph (its own 1024 coordinates; spans x 220..940, y 202..822), scaled and centred in the upper part.
  let s: CGFloat = 0.74
  let glyphW: CGFloat = 720
  cg.saveGState()
  cg.translateBy(x: size / 2 - (220 + glyphW / 2) * s, y: 110 - 202 * s)
  cg.scaleBy(x: s, y: s)
  cg.setFillColor(white)
  cg.setStrokeColor(white)

  // The phone: a white body with the screen cut out (the top edge is thicker, like the icon's).
  let body = CGPath(
    roundedRect: CGRect(x: 220, y: 202, width: 360, height: 620), cornerWidth: 75, cornerHeight: 75,
    transform: nil)
  let screen = CGPath(
    roundedRect: CGRect(x: 252, y: 262, width: 296, height: 520), cornerWidth: 45, cornerHeight: 45,
    transform: nil)
  cg.addPath(body)
  cg.addPath(screen)
  cg.fillPath(using: .evenOdd)

  // The dot, now the camera's lens: the icon's dot with a ring around it.
  cg.fillEllipse(in: CGRect(x: 400 - 52, y: 522 - 52, width: 104, height: 104))
  cg.setLineWidth(20)
  cg.setAlpha(0.55)
  cg.strokeEllipse(in: CGRect(x: 400 - 92, y: 522 - 92, width: 184, height: 184))
  cg.setAlpha(1)

  // The waves: three arcs around one centre, round caps, as in the icon.
  cg.setLineWidth(34)
  cg.setLineCap(.round)
  for (r, half) in [(94.0, 63.0), (193.0, 47.0), (293.0, 45.8)] {
    let a = half * .pi / 180
    cg.addArc(
      center: CGPoint(x: 645, y: 512), radius: r, startAngle: -a, endAngle: a, clockwise: false)
    cg.strokePath()
  }
  cg.restoreGState()

  // The words: what it buys, then for how long.
  func text(_ string: String, at y: CGFloat, size fontSize: CGFloat, weight: NSFont.Weight, alpha: CGFloat) {
    let style = NSMutableParagraphStyle()
    style.alignment = .center
    let attributes: [NSAttributedString.Key: Any] = [
      .font: NSFont.systemFont(ofSize: fontSize, weight: weight),
      .foregroundColor: NSColor(white: 1, alpha: alpha),
      .paragraphStyle: style,
      .kern: fontSize * -0.01
    ]
    let line = NSAttributedString(string: string, attributes: attributes)
    // The context is flipped to a top-left origin, so tell AppKit text it is drawing in a flipped one.
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(cgContext: cg, flipped: true)
    line.draw(in: CGRect(x: 0, y: y, width: size, height: fontSize * 1.3))
    NSGraphicsContext.restoreGraphicsState()
  }
  text(title, at: 650, size: 128, weight: .bold, alpha: 1)
  text(period, at: 800, size: 68, weight: .semibold, alpha: 0.82)

  let out = CGImageDestinationCreateWithURL(
    URL(fileURLWithPath: path) as CFURL, "public.png" as CFString, 1, nil)!
  CGImageDestinationAddImage(out, cg.makeImage()!, nil)
  precondition(CGImageDestinationFinalize(out))
}

let here = URL(fileURLWithPath: CommandLine.arguments[0]).deletingLastPathComponent().path
draw(title: "Camera", period: "Monthly", to: "\(here)/camera-monthly.png")
draw(title: "Camera", period: "6 months", to: "\(here)/camera-6-months.png")
