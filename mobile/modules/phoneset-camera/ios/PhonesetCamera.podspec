Pod::Spec.new do |s|
  s.name           = 'PhonesetCamera'
  s.version        = '1.0.0'
  s.summary        = 'The phone\'s camera as a source of JPEG frames for the phoneset protocol'
  s.description    = 'AVFoundation capture, one JPEG per frame, handed to JavaScript on request.'
  s.license        = 'MIT'
  s.author         = 'phoneset'
  s.homepage       = 'https://github.com/mehmetraufoguz/phoneset'
  s.platforms      = { :ios => '16.4' }
  s.swift_version  = '5.9'
  s.source         = { git: 'https://github.com/mehmetraufoguz/phoneset.git' }
  s.static_framework = true

  s.dependency 'ExpoModulesCore'

  s.pod_target_xcconfig = {
    'DEFINES_MODULE' => 'YES',
    'SWIFT_COMPILATION_MODE' => 'wholemodule'
  }
  s.source_files = '**/*.{h,m,swift}'
end
