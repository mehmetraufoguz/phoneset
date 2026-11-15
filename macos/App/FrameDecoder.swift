import CoreImage
import CoreMedia
import ImageIO
import VideoToolbox

/// Turns the coded frames of the stream into pictures: a JPEG with ImageIO, H.264 with a VideoToolbox decoder
/// (made again when the stream's parameter sets change). Not thread safe: use it from one queue.
final class FrameDecoder {
  /// Called with each decoded picture and the record it came from.
  var onImage: ((CIImage, FrameRecord) -> Void)?

  private var session: VTDecompressionSession?
  private var format: CMVideoFormatDescription?
  private var parameterSets: (sps: Data, pps: Data)?
  /// After a frame was dropped or failed, nothing is decoded until the next key frame.
  private var needsKey = true

  /// Makes the decoder wait for the next key frame (a frame was dropped in front of it).
  func lostFrame() { needsKey = true }

  func decode(_ record: FrameRecord) {
    switch record.codec {
    case FrameRecord.jpeg: decodeJPEG(record)
    case FrameRecord.h264: decodeH264(record)
    default: break
    }
  }

  private func decodeJPEG(_ record: FrameRecord) {
    guard
      let source = CGImageSourceCreateWithData(record.payload as CFData, nil),
      let image = CGImageSourceCreateImageAtIndex(source, 0, nil)
    else { return }
    onImage?(CIImage(cgImage: image), record)
  }

  private func decodeH264(_ record: FrameRecord) {
    let units = Self.nalUnits(of: record.payload)
    if record.key {
      let sps = units.first { $0.first.map { $0 & 0x1F } == 7 }
      let pps = units.first { $0.first.map { $0 & 0x1F } == 8 }
      if let sps, let pps, session == nil || parameterSets?.sps != sps || parameterSets?.pps != pps {
        makeSession(sps: sps, pps: pps)
      }
      if session != nil { needsKey = false }
    }
    guard !needsKey, let session, let format else { return }

    // The decoder takes each NAL unit behind a 4-byte length, not behind a start code; the parameter sets and the
    // access unit delimiter are left out.
    var data = Data()
    for unit in units {
      let type = (unit.first ?? 0) & 0x1F
      if type == 7 || type == 8 || type == 9 { continue }
      var length = UInt32(unit.count).bigEndian
      data.append(Data(bytes: &length, count: 4))
      data.append(unit)
    }
    guard !data.isEmpty else { return }

    var block: CMBlockBuffer?
    guard
      CMBlockBufferCreateWithMemoryBlock(
        allocator: nil, memoryBlock: nil, blockLength: data.count, blockAllocator: nil, customBlockSource: nil,
        offsetToData: 0, dataLength: data.count, flags: 0, blockBufferOut: &block) == kCMBlockBufferNoErr,
      let block,
      data.withUnsafeBytes({
        CMBlockBufferReplaceDataBytes(
          with: $0.baseAddress!, blockBuffer: block, offsetIntoDestination: 0, dataLength: data.count)
      }) == kCMBlockBufferNoErr
    else { return }

    var timing = CMSampleTimingInfo(
      duration: .invalid, presentationTimeStamp: CMTime(value: Int64(record.capturedMs), timescale: 1000),
      decodeTimeStamp: .invalid)
    var size = data.count
    var sample: CMSampleBuffer?
    guard
      CMSampleBufferCreateReady(
        allocator: nil, dataBuffer: block, formatDescription: format, sampleCount: 1, sampleTimingEntryCount: 1,
        sampleTimingArray: &timing, sampleSizeEntryCount: 1, sampleSizeArray: &size, sampleBufferOut: &sample)
        == noErr,
      let sample
    else { return }

    let status = VTDecompressionSessionDecodeFrame(session, sampleBuffer: sample, flags: [], infoFlagsOut: nil) {
      [weak self] status, _, image, _, _ in
      guard status == noErr, let image else { return }
      self?.onImage?(CIImage(cvPixelBuffer: image), record)
    }
    if status != noErr { needsKey = true }
  }

  private func makeSession(sps: Data, pps: Data) {
    if let session { VTDecompressionSessionInvalidate(session) }
    session = nil
    format = nil
    parameterSets = nil

    var description: CMVideoFormatDescription?
    let made = sps.withUnsafeBytes { spsBytes in
      pps.withUnsafeBytes { ppsBytes -> OSStatus in
        let pointers = [
          spsBytes.bindMemory(to: UInt8.self).baseAddress!, ppsBytes.bindMemory(to: UInt8.self).baseAddress!,
        ]
        let sizes = [sps.count, pps.count]
        return CMVideoFormatDescriptionCreateFromH264ParameterSets(
          allocator: nil, parameterSetCount: 2, parameterSetPointers: pointers, parameterSetSizes: sizes,
          nalUnitHeaderLength: 4, formatDescriptionOut: &description)
      }
    }
    guard made == noErr, let description else { return }

    var created: VTDecompressionSession?
    guard
      VTDecompressionSessionCreate(
        allocator: nil, formatDescription: description, decoderSpecification: nil, imageBufferAttributes: nil,
        outputCallback: nil, decompressionSessionOut: &created) == noErr,
      let created
    else { return }
    session = created
    format = description
    parameterSets = (sps, pps)
  }

  /// The NAL units of an Annex B access unit, without their start codes.
  static func nalUnits(of data: Data) -> [Data] {
    let bytes = [UInt8](data)
    var starts: [(code: Int, body: Int)] = []
    var i = 0
    while i + 2 < bytes.count {
      if bytes[i] == 0, bytes[i + 1] == 0, bytes[i + 2] == 1 {
        starts.append((i, i + 3))
        i += 3
      } else {
        i += 1
      }
    }
    var units: [Data] = []
    for (index, start) in starts.enumerated() {
      var end = index + 1 < starts.count ? starts[index + 1].code : bytes.count
      // A 4-byte start code (00 00 00 01) has one more zero in front of the 3-byte one.
      if index + 1 < starts.count, end > start.body, bytes[end - 1] == 0 { end -= 1 }
      if end > start.body { units.append(Data(bytes[start.body..<end])) }
    }
    return units
  }
}
