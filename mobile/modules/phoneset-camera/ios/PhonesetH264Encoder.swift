import CoreMedia
import Foundation
import VideoToolbox

/// A hardware H.264 coder for the camera's pictures (VideoToolbox). A picture goes in and, a moment later,
/// one access unit comes out in Annex B form (docs/protocol.md, section 8.3): NAL units each behind the
/// start code 00 00 00 01, and in front of a key frame the SPS and the PPS. There are no B-frames, so what
/// comes out is in the order it went in.
final class PhonesetH264Encoder {
  struct Unit {
    let data: Data
    let key: Bool
  }

  /// Called on a VideoToolbox thread, once for every picture handed to `encode`: with the unit, or with
  /// neither (the coder dropped the picture), or with a message (it failed).
  typealias Output = (Unit?, String?) -> Void

  private var session: VTCompressionSession?
  private let output: Output
  private static let startCode: [UInt8] = [0, 0, 0, 1]

  /// nil when the phone has no hardware coder for this size.
  init?(width: Int, height: Int, fps: Int, bitrate: Int, output: @escaping Output) {
    self.output = output
    // An iPhone codes H.264 in hardware by default; from iOS 17.4 it can be required, so that a phone
    // without one fails instead of coding in software.
    var specification: [String: Any] = [:]
    if #available(iOS 17.4, *) {
      specification[kVTVideoEncoderSpecification_EnableHardwareAcceleratedVideoEncoder as String] = true
      specification[kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder as String] = true
    }
    var created: VTCompressionSession?
    let status = VTCompressionSessionCreate(
      allocator: nil, width: Int32(width), height: Int32(height), codecType: kCMVideoCodecType_H264,
      encoderSpecification: specification as CFDictionary, imageBufferAttributes: nil,
      compressedDataAllocator: nil,
      outputCallback: { refcon, _, status, infoFlags, sampleBuffer in
        guard let refcon else { return }
        Unmanaged<PhonesetH264Encoder>.fromOpaque(refcon).takeUnretainedValue()
          .handle(status: status, infoFlags: infoFlags, sampleBuffer: sampleBuffer)
      },
      refcon: Unmanaged.passUnretained(self).toOpaque(), compressionSessionOut: &created)
    guard status == noErr, let created else { return nil }
    session = created

    set(kVTCompressionPropertyKey_RealTime, kCFBooleanTrue)
    set(kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_H264_High_AutoLevel)
    set(kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse)
    set(kVTCompressionPropertyKey_AverageBitRate, NSNumber(value: bitrate))
    // At most twice the average over any second, so one big picture can't flood the link.
    set(kVTCompressionPropertyKey_DataRateLimits, [NSNumber(value: bitrate / 8 * 2), NSNumber(value: 1)] as CFArray)
    set(kVTCompressionPropertyKey_ExpectedFrameRate, NSNumber(value: fps))
    // A key frame at least every 2 seconds (section 8.3).
    set(kVTCompressionPropertyKey_MaxKeyFrameInterval, NSNumber(value: fps * 2))
    set(kVTCompressionPropertyKey_MaxKeyFrameIntervalDuration, NSNumber(value: 2))
    VTCompressionSessionPrepareToEncodeFrames(created)
  }

  private func set(_ key: CFString, _ value: CFTypeRef) {
    if let session { VTSessionSetProperty(session, key: key, value: value) }
  }

  /// Hands a picture to the coder; the unit arrives through `output`. Returns false if it was refused.
  func encode(_ pixels: CVPixelBuffer, at time: CMTime, forceKey: Bool) -> Bool {
    guard let session else { return false }
    let properties: CFDictionary? =
      forceKey ? [kVTEncodeFrameOptionKey_ForceKeyFrame as String: true] as CFDictionary : nil
    return VTCompressionSessionEncodeFrame(
      session, imageBuffer: pixels, presentationTimeStamp: time, duration: .invalid,
      frameProperties: properties, sourceFrameRefcon: nil, infoFlagsOut: nil) == noErr
  }

  func invalidate() {
    if let session {
      VTCompressionSessionCompleteFrames(session, untilPresentationTimeStamp: .invalid)
      VTCompressionSessionInvalidate(session)
    }
    session = nil
  }

  private func handle(status: OSStatus, infoFlags: VTEncodeInfoFlags, sampleBuffer: CMSampleBuffer?) {
    if status != noErr {
      output(nil, "The video coder failed (\(status)).")
      return
    }
    guard let sampleBuffer, CMSampleBufferDataIsReady(sampleBuffer), !infoFlags.contains(.frameDropped) else {
      output(nil, nil)
      return
    }

    // A picture is a key frame unless the coder says it depends on an earlier one.
    var key = true
    if let attachments = CMSampleBufferGetSampleAttachmentsArray(sampleBuffer, createIfNecessary: false)
      as? [[CFString: Any]], let first = attachments.first
    {
      key = !(first[kCMSampleAttachmentKey_NotSync] as? Bool ?? false)
    }

    var data = Data()
    if key {
      guard let format = CMSampleBufferGetFormatDescription(sampleBuffer) else {
        output(nil, "The video coder gave no format.")
        return
      }
      for index in 0..<2 {  // 0 is the SPS and 1 the PPS
        var pointer: UnsafePointer<UInt8>?
        var size = 0
        let status = CMVideoFormatDescriptionGetH264ParameterSetAtIndex(
          format, parameterSetIndex: index, parameterSetPointerOut: &pointer, parameterSetSizeOut: &size,
          parameterSetCountOut: nil, nalUnitHeaderLengthOut: nil)
        guard status == noErr, let pointer else {
          output(nil, "The video coder gave no parameter sets.")
          return
        }
        data.append(contentsOf: Self.startCode)
        data.append(pointer, count: size)
      }
    }

    guard let block = CMSampleBufferGetDataBuffer(sampleBuffer) else {
      output(nil, "The video coder gave no data.")
      return
    }
    var length = 0
    var base: UnsafeMutablePointer<Int8>?
    guard
      CMBlockBufferGetDataPointer(block, atOffset: 0, lengthAtOffsetOut: nil, totalLengthOut: &length, dataPointerOut: &base)
        == kCMBlockBufferNoErr, let base
    else {
      output(nil, "The video coder's data could not be read.")
      return
    }
    // The coder writes each NAL unit behind a 4-byte length; Annex B wants a start code instead.
    var offset = 0
    while offset + 4 <= length {
      var big: UInt32 = 0
      memcpy(&big, base + offset, 4)
      let size = Int(UInt32(bigEndian: big))
      offset += 4
      guard size > 0, size <= length - offset else { break }
      data.append(contentsOf: Self.startCode)
      data.append(UnsafeRawPointer(base + offset).assumingMemoryBound(to: UInt8.self), count: size)
      offset += size
    }
    output(Unit(data: data, key: key), nil)
  }
}
