Pod::Spec.new do |s|
  s.name           = 'PhonesetCrypto'
  s.version        = '1.0.0'
  s.summary        = 'The operating system\'s own cryptography for the phoneset protocol'
  s.description    = 'P-256 ECDH, HKDF, HMAC, SHA-256 and AES-256-GCM through CryptoKit.'
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
