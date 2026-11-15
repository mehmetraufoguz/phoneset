import CoreImage
import CoreVideo

/// Turns a decoded picture into one frame of the camera: upright (the record's rotation and mirror), as large as it
/// fits in the output, with black bars at the sides or on top where the shape differs. Used from one queue.
final class Compositor {
  static let width = 1280
  static let height = 720

  private let context = CIContext()
  private var pool: CVPixelBufferPool?
  private let colorSpace = CGColorSpace(name: CGColorSpace.sRGB) ?? CGColorSpaceCreateDeviceRGB()

  init() {
    let attributes: [String: Any] = [
      kCVPixelBufferWidthKey as String: Self.width,
      kCVPixelBufferHeightKey as String: Self.height,
      kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA,
      kCVPixelBufferIOSurfacePropertiesKey as String: [:] as [String: Any],
    ]
    CVPixelBufferPoolCreate(nil, nil, attributes as CFDictionary, &pool)
  }

  func frame(from image: CIImage, rotation: Int, mirror: Bool) -> CVPixelBuffer? {
    guard let pool else { return nil }
    var picture = image
    // Core Image's y axis points up, so a turn clockwise is a negative angle.
    if rotation % 4 != 0 {
      picture = picture.transformed(by: CGAffineTransform(rotationAngle: -CGFloat(rotation % 4) * .pi / 2))
    }
    picture = picture.transformed(by: CGAffineTransform(translationX: -picture.extent.minX, y: -picture.extent.minY))
    if mirror {
      let width = picture.extent.width
      picture = picture.transformed(by: CGAffineTransform(scaleX: -1, y: 1).translatedBy(x: -width, y: 0))
    }

    let target = CGRect(x: 0, y: 0, width: Self.width, height: Self.height)
    let scale = min(target.width / picture.extent.width, target.height / picture.extent.height)
    picture = picture.transformed(by: CGAffineTransform(scaleX: scale, y: scale))
    picture = picture.transformed(
      by: CGAffineTransform(
        translationX: (target.width - picture.extent.width) / 2 - picture.extent.minX,
        y: (target.height - picture.extent.height) / 2 - picture.extent.minY))
    let framed = picture.composited(over: CIImage(color: .black).cropped(to: target))

    var pixels: CVPixelBuffer?
    guard CVPixelBufferPoolCreatePixelBuffer(nil, pool, &pixels) == kCVReturnSuccess, let pixels else { return nil }
    context.render(framed, to: pixels, bounds: target, colorSpace: colorSpace)
    return pixels
  }
}
